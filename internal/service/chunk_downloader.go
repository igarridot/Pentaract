package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/repository"
)

func (m *StorageManager) downloadParallelism(ctx context.Context, storageID uuid.UUID, chunksCount int) int {
	if workers := m.preferredDownloadWorkers(ctx, storageID, chunksCount); len(workers) > 0 {
		n := len(workers)
		if n > DownloadChunkParallelism {
			n = DownloadChunkParallelism
		}
		return n
	}
	return 1
}

func (m *StorageManager) downloadChunkWithWorker(ctx context.Context, storage domain.Storage, chunk domain.FileChunk, wt repository.WorkerToken) ([]byte, error) {
	data, err := m.tgClient.Download(ctx, wt.Token, chunk.TelegramFileID)
	if err == nil {
		return data, nil
	}
	if !isGetFileFailure(err) || chunk.TelegramMessageID == 0 {
		return nil, err
	}

	fileID, resolveErr := m.tgClient.ResolveFileIDByMessage(ctx, wt.Token, storage.ChatID, chunk.TelegramMessageID)
	if resolveErr != nil {
		return nil, fmt.Errorf("%w: %v (original: %v)", domain.ErrTelegramResolveFailed, resolveErr, err)
	}

	data, err = m.tgClient.Download(ctx, wt.Token, fileID)
	if err != nil {
		return nil, err
	}

	if fileID != chunk.TelegramFileID && chunk.ID != uuid.Nil {
		if updateErr := m.filesRepo.UpdateChunkTelegramFileID(ctx, chunk.ID, fileID); updateErr != nil {
			slog.Warn("failed updating chunk file_id", "position", chunk.Position, "err", updateErr)
		}
	}

	return data, nil
}

func (m *StorageManager) preferredDownloadWorkers(ctx context.Context, storageID uuid.UUID, chunksCount int) []repository.WorkerToken {
	if chunksCount <= 1 || m.workersRepo == nil {
		return nil
	}

	workers, err := m.workersRepo.ListTokensByStorage(ctx, storageID)
	if err != nil {
		slog.Warn("failed listing workers for download, falling back to scheduler-only selection", "storage_id", storageID, "err", err)
		return nil
	}
	if len(workers) <= 1 {
		return nil
	}
	if len(workers) > chunksCount {
		workers = workers[:chunksCount]
	}
	return workers
}

// ListDownloadWorkers returns all worker tokens assigned to a storage.
func (m *StorageManager) ListDownloadWorkers(ctx context.Context, storageID uuid.UUID) ([]repository.WorkerToken, error) {
	if m.workersRepo == nil {
		return nil, nil
	}

	return m.workersRepo.ListTokensByStorage(ctx, storageID)
}

func (m *StorageManager) downloadChunk(ctx context.Context, storage domain.Storage, chunk domain.FileChunk) ([]byte, error) {
	wt, err := m.scheduler.GetToken(ctx, storage.ID)
	if err != nil {
		return nil, fmt.Errorf("getting token for chunk %d: %w", chunk.Position, err)
	}

	data, err := m.downloadChunkWithWorker(ctx, storage, chunk, *wt)
	if err == nil {
		return data, nil
	}
	if contextAborted(ctx, err) {
		return nil, contextAbortError(ctx, err)
	}
	if !isGetFileFailure(err) {
		return nil, err
	}

	workers, listErr := m.workersRepo.ListTokensByStorage(ctx, storage.ID)
	if listErr != nil {
		return nil, fmt.Errorf("fallback workers lookup failed after getFile error: %w", listErr)
	}

	lastErr := err
	for _, candidate := range workers {
		if candidate.Token == wt.Token {
			continue
		}
		data, tryErr := m.downloadChunkWithWorker(ctx, storage, chunk, candidate)
		if tryErr == nil {
			slog.Info("chunk recovered via fallback worker", "position", chunk.Position, "worker", candidate.Name)
			return data, nil
		}
		if contextAborted(ctx, tryErr) {
			return nil, contextAbortError(ctx, tryErr)
		}
		lastErr = tryErr
	}

	return nil, lastErr
}

func (m *StorageManager) downloadChunkWithPreferredWorker(ctx context.Context, storage domain.Storage, chunk domain.FileChunk, preferredWorker *repository.WorkerToken) ([]byte, error) {
	if preferredWorker != nil {
		data, err := m.downloadChunkWithWorker(ctx, storage, chunk, *preferredWorker)
		if err == nil {
			return data, nil
		}
		if contextAborted(ctx, err) {
			return nil, contextAbortError(ctx, err)
		}
		slog.Warn("preferred worker failed for chunk, falling back", "worker", preferredWorker.Name, "position", chunk.Position, "err", err)
	}

	return m.downloadChunk(ctx, storage, chunk)
}

// downloadChunkWithRetry wraps downloadChunkWithPreferredWorker with
// retry + exponential backoff. Each attempt already includes worker fallback
// logic via downloadChunk.
func (m *StorageManager) downloadChunkWithRetry(ctx context.Context, storage domain.Storage, chunk domain.FileChunk, preferredWorker *repository.WorkerToken) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= DownloadChunkMaxAttempts; attempt++ {
		data, err := m.downloadChunkWithPreferredWorker(ctx, storage, chunk, preferredWorker)
		if err == nil {
			return data, nil
		}
		if contextAborted(ctx, err) {
			return nil, contextAbortError(ctx, err)
		}
		lastErr = err
		if attempt < DownloadChunkMaxAttempts {
			slog.Warn("download chunk failed, retrying", "position", chunk.Position, "attempt", attempt, "max_attempts", DownloadChunkMaxAttempts, "err", err)
			backoff := time.Duration(attempt) * 500 * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
	}
	return nil, lastErr
}

func (m *StorageManager) downloadAndDecryptChunk(ctx context.Context, fileID uuid.UUID, storage domain.Storage, chunk domain.FileChunk, preferredWorker *repository.WorkerToken) ([]byte, error) {
	data, err := m.downloadChunkWithRetry(ctx, storage, chunk, preferredWorker)
	if err != nil {
		return nil, fmt.Errorf("downloading chunk %d: %w", chunk.Position, err)
	}
	data, err = m.chunkCipher.DecryptChunk(fileID, chunk.Position, data)
	if err != nil {
		return nil, fmt.Errorf("decrypting chunk %d: %w", chunk.Position, err)
	}
	return data, nil
}

func (m *StorageManager) downloadAndDecryptChunkCached(ctx context.Context, fileID uuid.UUID, storage domain.Storage, chunk domain.FileChunk, preferredWorker *repository.WorkerToken) ([]byte, error) {
	cacheKey := streamChunkCacheKey{fileID: fileID, position: chunk.Position}
	cache := m.getStreamChunkCache()
	if data, ok := cache.get(cacheKey); ok {
		return data, nil
	}

	loadKey := fmt.Sprintf("%s:%d", fileID.String(), chunk.Position)
	value, err, _ := m.streamChunkLoads.Do(loadKey, func() (interface{}, error) {
		if data, ok := cache.get(cacheKey); ok {
			return data, nil
		}

		data, err := m.downloadAndDecryptChunk(ctx, fileID, storage, chunk, preferredWorker)
		if err != nil {
			return nil, err
		}

		cache.set(cacheKey, data)
		return data, nil
	})
	if err != nil {
		return nil, err
	}

	data, ok := value.([]byte)
	if !ok {
		return nil, fmt.Errorf("unexpected cached chunk type %T", value)
	}

	return data, nil
}

type chunkDataLoader func(ctx context.Context, fileID uuid.UUID, storage domain.Storage, chunk domain.FileChunk, preferredWorker *repository.WorkerToken) ([]byte, error)

type chunkDownloadJob struct {
	chunk           domain.FileChunk
	preferredWorker *repository.WorkerToken
}

func (m *StorageManager) downloadChunksInOrderWithLoaderAndWorkers(
	ctx context.Context,
	file *domain.File,
	storage *domain.Storage,
	chunks []domain.FileChunk,
	loadChunk chunkDataLoader,
	writeChunk func(chunk domain.FileChunk, data []byte) error,
	preferredWorkers []repository.WorkerToken,
) error {
	if len(preferredWorkers) == 0 {
		preferredWorkers = m.preferredDownloadWorkers(ctx, storage.ID, len(chunks))
	}
	parallelism := len(preferredWorkers)
	if parallelism <= 1 {
		if len(preferredWorkers) == 0 {
			preferredWorkers = nil
		}
		parallelism = 1
	}
	if parallelism > DownloadChunkParallelism {
		parallelism = DownloadChunkParallelism
	}

	jobs := make([]chunkDownloadJob, len(chunks))
	for index, chunk := range chunks {
		job := chunkDownloadJob{chunk: chunk}
		if len(preferredWorkers) > 0 {
			worker := preferredWorkers[index%len(preferredWorkers)]
			job.preferredWorker = &worker
		}
		jobs[index] = job
	}

	return runOrderedJobs(
		ctx,
		parallelism,
		jobs,
		func(loadCtx context.Context, job chunkDownloadJob) ([]byte, error) {
			return loadChunk(loadCtx, file.ID, *storage, job.chunk, job.preferredWorker)
		},
		func(job chunkDownloadJob, data []byte) error {
			return writeChunk(job.chunk, data)
		},
		nil,
	)
}

func (m *StorageManager) downloadChunksInOrder(
	ctx context.Context,
	file *domain.File,
	storage *domain.Storage,
	chunks []domain.FileChunk,
	writeChunk func(chunk domain.FileChunk, data []byte) error,
) error {
	return m.downloadChunksInOrderWithLoaderAndWorkers(ctx, file, storage, chunks, m.downloadAndDecryptChunkCached, writeChunk, nil)
}

// DownloadToWriter streams a file's chunks sequentially to the given writer.
// This path is intentionally conservative and avoids stream-specific caching so
// regular file downloads do not keep extra decrypted chunks resident in memory.
// Pass nil for preferredWorkers to let the manager select workers automatically.
func (m *StorageManager) DownloadToWriter(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress, preferredWorkers []repository.WorkerToken) error {
	chunks, err := m.filesRepo.ListChunks(ctx, file.ID)
	if err != nil {
		return fmt.Errorf("listing chunks: %w", err)
	}

	if len(chunks) == 0 {
		return domain.ErrNotFound("file chunks")
	}

	if progress != nil && progress.TotalChunks == 0 && progress.TotalBytes == 0 {
		progress.TotalChunks = int64(len(chunks))
		progress.TotalBytes = file.Size
	}

	storage, err := m.storagesRepo.GetByID(ctx, file.StorageID)
	if err != nil {
		return fmt.Errorf("getting storage: %w", err)
	}

	slog.Info("starting download", "file", file.Path, "chunks", len(chunks), "storage", storage.Name, "chat", storage.Name)

	if err := m.downloadChunksInOrderWithLoaderAndWorkers(ctx, file, storage, chunks, m.downloadAndDecryptChunk, func(chunk domain.FileChunk, data []byte) error {
		if _, err := w.Write(data); err != nil {
			return fmt.Errorf("writing chunk %d: %w", chunk.Position, err)
		}

		if progress != nil {
			progress.DownloadedChunks.Add(1)
			progress.DownloadedBytes.Add(int64(len(data)))
		}
		return nil
	}, preferredWorkers); err != nil {
		return err
	}

	slog.Info("download completed", "file", file.Path, "storage", storage.Name, "chat", storage.Name)
	return nil
}

// StreamToWriter keeps the optimized streaming path used by inline previews and
// media playback. It can reuse cached decrypted chunks and download ahead in
// parallel, which helps with buffering and seek-heavy clients such as Kodi.
func (m *StorageManager) StreamToWriter(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress) error {
	chunks, err := m.filesRepo.ListChunks(ctx, file.ID)
	if err != nil {
		return fmt.Errorf("listing chunks: %w", err)
	}

	if len(chunks) == 0 {
		return domain.ErrNotFound("file chunks")
	}

	if progress != nil && progress.TotalChunks == 0 && progress.TotalBytes == 0 {
		progress.TotalChunks = int64(len(chunks))
		progress.TotalBytes = file.Size
	}

	storage, err := m.storagesRepo.GetByID(ctx, file.StorageID)
	if err != nil {
		return fmt.Errorf("getting storage: %w", err)
	}

	slog.Info("starting stream", "file", file.Path, "chunks", len(chunks), "storage", storage.Name, "chat", storage.Name)

	if err := m.downloadChunksInOrder(ctx, file, storage, chunks, func(chunk domain.FileChunk, data []byte) error {
		if _, err := w.Write(data); err != nil {
			return fmt.Errorf("writing chunk %d: %w", chunk.Position, err)
		}

		if progress != nil {
			progress.DownloadedChunks.Add(1)
			progress.DownloadedBytes.Add(int64(len(data)))
		}
		return nil
	}); err != nil {
		return err
	}

	slog.Info("stream completed", "file", file.Path, "storage", storage.Name, "chat", storage.Name)
	return nil
}

// ExactFileSize derives the exact file size from chunk count plus the actual
// plaintext bytes in the last chunk. Current uploads always use UploadChunkSize
// for every non-final chunk, so this avoids downloading the full file before a
// stream can even start.
func (m *StorageManager) ExactFileSize(ctx context.Context, file *domain.File) (int64, error) {
	chunks, err := m.filesRepo.ListChunks(ctx, file.ID)
	if err != nil {
		return 0, fmt.Errorf("listing chunks: %w", err)
	}
	if len(chunks) == 0 {
		return 0, domain.ErrNotFound("file chunks")
	}

	storage, err := m.storagesRepo.GetByID(ctx, file.StorageID)
	if err != nil {
		return 0, fmt.Errorf("getting storage: %w", err)
	}

	lastChunk := chunks[len(chunks)-1]
	lastChunkData, err := m.downloadAndDecryptChunkCached(ctx, file.ID, *storage, lastChunk, nil)
	if err != nil {
		return 0, err
	}

	total := int64(len(lastChunkData))
	if len(chunks) > 1 {
		total += int64(len(chunks)-1) * UploadChunkSize
	}

	return total, nil
}

func inferRangeChunkWindow(start, end, totalSize int64, chunksCount int) (int, int, int64, bool) {
	if chunksCount <= 0 {
		return 0, 0, 0, false
	}
	if chunksCount == 1 {
		return 0, 0, 0, true
	}

	// Current uploads use a fixed plaintext size for every non-final chunk.
	// When that invariant holds, we can jump straight to the chunk that contains
	// the requested byte range instead of replaying the whole file from byte 0.
	fullPrefixSize := int64(chunksCount-1) * UploadChunkSize
	if totalSize < fullPrefixSize {
		return 0, chunksCount - 1, 0, false
	}

	lastChunkIndex := chunksCount - 1
	startChunkIndex := int(start / UploadChunkSize)
	endChunkIndex := int(end / UploadChunkSize)
	if startChunkIndex > lastChunkIndex {
		startChunkIndex = lastChunkIndex
	}
	if endChunkIndex > lastChunkIndex {
		endChunkIndex = lastChunkIndex
	}

	return startChunkIndex, endChunkIndex, int64(startChunkIndex) * UploadChunkSize, true
}

// DownloadRangeToWriter streams only the requested byte range [start, end] (inclusive).
// It downloads only the needed Telegram chunks and trims boundaries in memory.
func (m *StorageManager) DownloadRangeToWriter(ctx context.Context, file *domain.File, w io.Writer, start, end, totalSize int64, progress *DownloadProgress) error {
	if start < 0 || end < start || end >= totalSize {
		return fmt.Errorf("invalid range %d-%d for file size %d", start, end, totalSize)
	}

	chunks, err := m.filesRepo.ListChunks(ctx, file.ID)
	if err != nil {
		return fmt.Errorf("listing chunks: %w", err)
	}
	if len(chunks) == 0 {
		return domain.ErrNotFound("file chunks")
	}

	storage, err := m.storagesRepo.GetByID(ctx, file.StorageID)
	if err != nil {
		return fmt.Errorf("getting storage: %w", err)
	}

	if progress != nil && progress.TotalBytes == 0 {
		progress.TotalBytes = end - start + 1
	}

	startChunkIndex := 0
	endChunkIndex := len(chunks) - 1
	offset := int64(0)
	if inferredStart, inferredEnd, inferredOffset, ok := inferRangeChunkWindow(start, end, totalSize, len(chunks)); ok {
		startChunkIndex = inferredStart
		endChunkIndex = inferredEnd
		offset = inferredOffset
	}

	rangeChunks := chunks[startChunkIndex : endChunkIndex+1]
	if progress != nil && progress.TotalChunks == 0 {
		progress.TotalChunks = int64(len(rangeChunks))
	}

	return m.downloadChunksInOrder(ctx, file, storage, rangeChunks, func(chunk domain.FileChunk, data []byte) error {
		chunkStart := offset
		chunkEndExclusive := chunkStart + int64(len(data))
		offset = chunkEndExclusive

		if chunkEndExclusive <= start || chunkStart > end {
			return nil
		}

		left := int64(0)
		if start > chunkStart {
			left = start - chunkStart
		}
		right := int64(len(data))
		if end+1 < chunkEndExclusive {
			right = end + 1 - chunkStart
		}
		if left < 0 {
			left = 0
		}
		if right > int64(len(data)) {
			right = int64(len(data))
		}
		if left >= right {
			return nil
		}

		if _, err := w.Write(data[left:right]); err != nil {
			return fmt.Errorf("writing ranged chunk %d: %w", chunk.Position, err)
		}

		if progress != nil {
			progress.DownloadedChunks.Add(1)
			progress.DownloadedBytes.Add(right - left)
		}

		return nil
	})
}
