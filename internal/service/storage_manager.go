package service

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/repository"
	"github.com/Dominux/Pentaract/internal/telegram"
)

const (
	maxTelegramGetFileBytes = 20 * 1024 * 1024
	uploadChunkSafetyMargin = 64 * 1024 // keep encrypted chunk comfortably below 20MB
	uploadChunkSize         = maxTelegramGetFileBytes - uploadChunkSafetyMargin
)

type StorageManager struct {
	filesRepo    *repository.FilesRepo
	storagesRepo *repository.StoragesRepo
	workersRepo  *repository.StorageWorkersRepo
	scheduler    *WorkerScheduler
	tgClient     *telegram.Client
	chunkCipher  *ChunkCipher
}

func NewStorageManager(
	filesRepo *repository.FilesRepo,
	storagesRepo *repository.StoragesRepo,
	workersRepo *repository.StorageWorkersRepo,
	scheduler *WorkerScheduler,
	tgClient *telegram.Client,
	encryptionSecret string,
) *StorageManager {
	return &StorageManager{
		filesRepo:    filesRepo,
		storagesRepo: storagesRepo,
		workersRepo:  workersRepo,
		scheduler:    scheduler,
		tgClient:     tgClient,
		chunkCipher:  NewChunkCipher(encryptionSecret),
	}
}

func isGetFileFailure(err error) bool {
	return err != nil && strings.Contains(err.Error(), "telegram getFile failed")
}

func isTelegramFileTooBig(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "file is too big")
}

func validateEncryptedChunkSize(chunk []byte) error {
	if len(chunk) > maxTelegramGetFileBytes {
		return fmt.Errorf("encrypted chunk size %d exceeds Telegram Bot API getFile limit %d", len(chunk), maxTelegramGetFileBytes)
	}
	return nil
}

func (m *StorageManager) downloadParallelism(ctx context.Context, storageID uuid.UUID, chunksCount int) int {
	if chunksCount <= 1 || m.workersRepo == nil {
		return 1
	}

	workers, err := m.workersRepo.ListTokensByStorage(ctx, storageID)
	if err != nil {
		log.Printf("[download] warning: failed listing workers for storage %s, falling back to sequential download: %v", storageID, err)
		return 1
	}
	if len(workers) <= 1 {
		return 1
	}
	if len(workers) > chunksCount {
		return chunksCount
	}
	return len(workers)
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
		return nil, fmt.Errorf("%w; resolving file_id from message failed: %v", err, resolveErr)
	}

	data, err = m.tgClient.Download(ctx, wt.Token, fileID)
	if err != nil {
		return nil, err
	}

	if fileID != chunk.TelegramFileID && chunk.ID != uuid.Nil {
		if updateErr := m.filesRepo.UpdateChunkTelegramFileID(ctx, chunk.ID, fileID); updateErr != nil {
			log.Printf("[download] warning: failed updating chunk %d file_id: %v", chunk.Position, updateErr)
		}
	}

	return data, nil
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
			log.Printf("[download] chunk %d recovered via fallback worker=%s", chunk.Position, candidate.Name)
			return data, nil
		}
		lastErr = tryErr
	}

	return nil, lastErr
}

func (m *StorageManager) downloadAndDecryptChunk(ctx context.Context, fileID uuid.UUID, storage domain.Storage, chunk domain.FileChunk) ([]byte, error) {
	data, err := m.downloadChunk(ctx, storage, chunk)
	if err != nil {
		if isTelegramFileTooBig(err) {
			return nil, fmt.Errorf("downloading chunk %d: telegram chunk exceeds Bot API download limit (20MB). re-upload file with smaller chunk size", chunk.Position)
		}
		return nil, fmt.Errorf("downloading chunk %d: %w", chunk.Position, err)
	}
	data, err = m.chunkCipher.DecryptChunk(fileID, chunk.Position, data)
	if err != nil {
		return nil, fmt.Errorf("decrypting chunk %d: %w", chunk.Position, err)
	}
	return data, nil
}

// UploadProgress tracks chunk upload progress.
type UploadProgress struct {
	TotalChunks    int32
	UploadedChunks atomic.Int32
	TotalBytes     int64
	UploadedBytes  atomic.Int64
}

// DownloadProgress tracks chunk download progress.
type DownloadProgress struct {
	TotalChunks      int64
	DownloadedChunks atomic.Int64
	TotalBytes       int64
	DownloadedBytes  atomic.Int64
}

// DeleteProgress tracks chunk deletion progress in Telegram.
type DeleteProgress struct {
	TotalChunks   int64
	DeletedChunks atomic.Int64
}

// Upload reads from reader chunk by chunk (streaming), uploads to Telegram in parallel,
// and records chunk metadata in the DB. Never holds the full file in memory.
func (m *StorageManager) Upload(ctx context.Context, file *domain.File, reader io.Reader, progress *UploadProgress) error {
	storage, err := m.storagesRepo.GetByID(ctx, file.StorageID)
	if err != nil {
		return fmt.Errorf("getting storage: %w", err)
	}

	log.Printf("[upload] starting file=%s storage=%s (chat=%s)", file.Path, storage.Name, storage.Name)

	// Calculate total chunks from file size if known
	if progress != nil && progress.TotalBytes > 0 {
		total := int32(progress.TotalBytes / uploadChunkSize)
		if progress.TotalBytes%uploadChunkSize != 0 {
			total++
		}
		progress.TotalChunks = total
	}

	type chunkResult struct {
		TelegramFileID    string
		TelegramMessageID int64
		Position          int16
	}

	var mu sync.Mutex
	var results []chunkResult

	// cleanupResults deletes any chunks already uploaded to Telegram.
	cleanupResults := func() {
		mu.Lock()
		uploaded := make([]chunkResult, len(results))
		copy(uploaded, results)
		mu.Unlock()

		if len(uploaded) > 0 {
			log.Printf("[upload] cancelled/failed file=%s, cleaning up %d chunks from telegram", file.Path, len(uploaded))
			cleanupChunks := make([]domain.FileChunk, len(uploaded))
			for i, r := range uploaded {
				cleanupChunks[i] = domain.FileChunk{
					TelegramMessageID: r.TelegramMessageID,
				}
			}
			go func() {
				if err := m.DeleteFromTelegram(context.Background(), *storage, cleanupChunks, nil); err != nil {
					log.Printf("[delete] WARNING: cleanup after failed upload returned error: %v", err)
				}
			}()
		}
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(5)

	// Close the reader when context is cancelled so io.ReadFull unblocks.
	cancelled := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			if rc, ok := reader.(io.Closer); ok {
				rc.Close()
			}
		case <-cancelled:
		}
	}()
	defer close(cancelled)

	position := int16(0)
	var readCancelled bool
	for {
		// Check context before blocking on read
		select {
		case <-ctx.Done():
			readCancelled = true
		default:
		}
		if readCancelled {
			break
		}

		buf := make([]byte, uploadChunkSize)
		n, readErr := io.ReadFull(reader, buf)
		if n == 0 && readErr != nil {
			// Check if the read failed due to cancellation
			if ctx.Err() != nil {
				readCancelled = true
			}
			break
		}
		chunkData := buf[:n]
		pos := position
		position++

		g.Go(func() error {
			wt, err := m.scheduler.GetToken(gctx, storage.ID)
			if err != nil {
				return fmt.Errorf("getting token for chunk %d: %w", pos, err)
			}

			log.Printf("[upload] chunk %d of file=%s via worker=%s chat=%s", pos, file.Path, wt.Name, storage.Name)

			encryptedChunkData, err := m.chunkCipher.EncryptChunk(file.ID, pos, chunkData)
			if err != nil {
				return fmt.Errorf("encrypting chunk %d: %w", pos, err)
			}
			if err := validateEncryptedChunkSize(encryptedChunkData); err != nil {
				return fmt.Errorf("validating encrypted chunk %d size: %w", pos, err)
			}

			filename := telegram.GenerateChunkFilename(file.ID, int(pos))
			result, err := m.tgClient.Upload(wt.Token, storage.ChatID, encryptedChunkData, filename)
			if err != nil {
				return fmt.Errorf("uploading chunk %d: %w", pos, err)
			}

			mu.Lock()
			results = append(results, chunkResult{
				TelegramFileID:    result.FileID,
				TelegramMessageID: result.MessageID,
				Position:          pos,
			})
			mu.Unlock()

			if progress != nil {
				progress.UploadedChunks.Add(1)
				progress.UploadedBytes.Add(int64(len(chunkData)))
			}

			return nil
		})

		if readErr != nil {
			if ctx.Err() != nil {
				readCancelled = true
			}
			break
		}
	}

	if progress != nil {
		progress.TotalChunks = int32(position)
	}

	waitErr := g.Wait()

	// If cancelled or failed, clean up uploaded chunks from Telegram
	if readCancelled || waitErr != nil {
		cleanupResults()
		if waitErr != nil {
			return waitErr
		}
		return ctx.Err()
	}

	// Create chunk records
	fileChunks := make([]domain.FileChunk, len(results))
	for i, r := range results {
		fileChunks[i] = domain.FileChunk{
			FileID:            file.ID,
			TelegramFileID:    r.TelegramFileID,
			TelegramMessageID: r.TelegramMessageID,
			Position:          r.Position,
		}
	}

	if err := m.filesRepo.CreateChunks(ctx, fileChunks); err != nil {
		cleanupResults()
		return fmt.Errorf("saving chunks: %w", err)
	}

	if err := m.filesRepo.MarkUploaded(ctx, file.ID); err != nil {
		return fmt.Errorf("marking file as uploaded: %w", err)
	}

	log.Printf("[upload] completed file=%s chunks=%d storage=%s chat=%s", file.Path, len(results), storage.Name, storage.Name)

	return nil
}

// DeleteFromTelegram deletes chunk messages from Telegram for the given chunks.
func (m *StorageManager) DeleteFromTelegram(ctx context.Context, storage domain.Storage, chunks []domain.FileChunk, progress *DeleteProgress) error {
	log.Printf("[delete] removing %d chunks from telegram chat=%s", len(chunks), storage.Name)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(5)

	if progress != nil {
		total := int64(0)
		for _, c := range chunks {
			if c.TelegramMessageID != 0 {
				total++
			}
		}
		progress.TotalChunks = total
	}

	for _, c := range chunks {
		if c.TelegramMessageID == 0 {
			continue
		}
		g.Go(func() error {
			wt, err := m.scheduler.GetToken(gctx, storage.ID)
			if err != nil {
				return fmt.Errorf("getting token for delete message %d: %w", c.TelegramMessageID, err)
			}
			log.Printf("[delete] deleting message %d via worker=%s chat=%s", c.TelegramMessageID, wt.Name, storage.Name)
			if err := m.tgClient.DeleteMessage(wt.Token, storage.ChatID, c.TelegramMessageID); err != nil {
				return fmt.Errorf("deleting message %d via worker=%s: %w", c.TelegramMessageID, wt.Name, err)
			}
			if progress != nil {
				progress.DeletedChunks.Add(1)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}
	log.Printf("[delete] finished removing chunks from chat=%s", storage.Name)
	return nil
}

type downloadedChunkResult struct {
	index int
	chunk domain.FileChunk
	data  []byte
}

func (m *StorageManager) downloadChunksInOrder(
	ctx context.Context,
	file *domain.File,
	storage *domain.Storage,
	chunks []domain.FileChunk,
	writeChunk func(chunk domain.FileChunk, data []byte) error,
) error {
	parallelism := m.downloadParallelism(ctx, storage.ID, len(chunks))
	if parallelism <= 1 {
		for _, chunk := range chunks {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			data, err := m.downloadAndDecryptChunk(ctx, file.ID, *storage, chunk)
			if err != nil {
				return err
			}
			if err := writeChunk(chunk, data); err != nil {
				return err
			}
		}
		return nil
	}

	type chunkJob struct {
		index int
		chunk domain.FileChunk
	}

	downloadCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	g, gctx := errgroup.WithContext(downloadCtx)
	jobs := make(chan chunkJob)
	results := make(chan downloadedChunkResult, parallelism)

	for range parallelism {
		g.Go(func() error {
			for job := range jobs {
				data, err := m.downloadAndDecryptChunk(gctx, file.ID, *storage, job.chunk)
				if err != nil {
					return err
				}

				select {
				case results <- downloadedChunkResult{index: job.index, chunk: job.chunk, data: data}:
				case <-gctx.Done():
					return gctx.Err()
				}
			}
			return nil
		})
	}

	g.Go(func() error {
		defer close(jobs)
		for i, chunk := range chunks {
			select {
			case jobs <- chunkJob{index: i, chunk: chunk}:
			case <-gctx.Done():
				return gctx.Err()
			}
		}
		return nil
	})

	errCh := make(chan error, 1)
	go func() {
		err := g.Wait()
		close(results)
		errCh <- err
	}()

	pending := make(map[int]downloadedChunkResult, parallelism)
	nextIndex := 0
	var writeErr error

	for result := range results {
		pending[result.index] = result

		for {
			next, ok := pending[nextIndex]
			if !ok {
				break
			}
			delete(pending, nextIndex)

			if writeErr == nil {
				if err := writeChunk(next.chunk, next.data); err != nil {
					writeErr = err
					cancel()
				}
			}
			nextIndex++
		}
	}

	downloadErr := <-errCh
	if writeErr != nil {
		return writeErr
	}
	if downloadErr != nil && downloadErr != context.Canceled {
		return downloadErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// DownloadToWriter streams a file's chunks sequentially to the given writer.
// Chunks are downloaded in parallel and written in order.
// Peak memory is bounded by roughly one downloaded chunk per available worker.
func (m *StorageManager) DownloadToWriter(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress) error {
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

	log.Printf("[download] starting file=%s chunks=%d storage=%s chat=%s", file.Path, len(chunks), storage.Name, storage.Name)

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

	log.Printf("[download] completed file=%s storage=%s chat=%s", file.Path, storage.Name, storage.Name)
	return nil
}

// ExactFileSize derives the exact file size using chunk count and actual last-chunk bytes.
// This avoids relying on persisted file.Size when legacy rows contain multipart envelope overhead.
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

	var total int64
	for _, chunk := range chunks {
		data, err := m.downloadAndDecryptChunk(ctx, file.ID, *storage, chunk)
		if err != nil {
			return 0, err
		}
		total += int64(len(data))
	}
	return total, nil
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

	offset := int64(0)
	for _, chunk := range chunks {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		data, err := m.downloadAndDecryptChunk(ctx, file.ID, *storage, chunk)
		if err != nil {
			return err
		}

		chunkStart := offset
		chunkEndExclusive := chunkStart + int64(len(data))
		offset = chunkEndExclusive

		if chunkEndExclusive <= start {
			continue
		}
		if chunkStart > end {
			break
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
			continue
		}

		if _, err := w.Write(data[left:right]); err != nil {
			return fmt.Errorf("writing ranged chunk %d: %w", chunk.Position, err)
		}

		if progress != nil {
			progress.DownloadedChunks.Add(1)
			progress.DownloadedBytes.Add(right - left)
		}

		if chunkEndExclusive > end {
			break
		}
	}

	return nil
}
