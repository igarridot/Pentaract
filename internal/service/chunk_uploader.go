package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/telegram"
)

var uploadChunkBufferPool = sync.Pool{
	New: func() any {
		return make([]byte, UploadChunkSize)
	},
}

// failedChunkInfo holds information about a chunk that failed during the
// parallel upload phase so it can be retried in a sequential second round.
type failedChunkInfo struct {
	position  int16
	data      []byte // independent copy — pool buffer is reused
	plainHash [sha256.Size]byte
}

func shouldRetryChunkUpload(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	return !contextAborted(ctx, err)
}

func (m *StorageManager) uploadChunkWithRetry(ctx context.Context, file *domain.File, storage *domain.Storage, position int16, chunkData []byte, plainHash [sha256.Size]byte) (uploadedChunkResult, error) {
	encryptedChunkData, err := m.chunkCipher.EncryptChunk(file.ID, position, chunkData)
	if err != nil {
		return uploadedChunkResult{}, fmt.Errorf("encrypting chunk %d: %w", position, err)
	}
	if err := validateEncryptedChunkSize(encryptedChunkData); err != nil {
		return uploadedChunkResult{}, fmt.Errorf("validating encrypted chunk %d size: %w", position, err)
	}

	filename := telegram.GenerateChunkFilename(file.ID, int(position))
	for attempt := 1; attempt <= UploadChunkMaxAttempts; attempt++ {
		wt, err := m.scheduler.GetToken(ctx, storage.ID)
		if err != nil {
			return uploadedChunkResult{}, fmt.Errorf("getting token for chunk %d: %w", position, err)
		}

		slog.Info("uploading chunk", "position", position, "file", file.Path, "worker", wt.Name, "storage", storage.Name, "attempt", attempt, "max_attempts", UploadChunkMaxAttempts)

		result, err := m.tgClient.Upload(wt.Token, storage.ChatID, encryptedChunkData, filename)
		if err == nil {
			return uploadedChunkResult{
				TelegramFileID:    result.FileID,
				TelegramMessageID: result.MessageID,
				Position:          position,
				PlainHash:         plainHash,
			}, nil
		}

		if !shouldRetryChunkUpload(ctx, err) || attempt == UploadChunkMaxAttempts {
			return uploadedChunkResult{}, fmt.Errorf("uploading chunk %d: %w", position, err)
		}

		slog.Warn("chunk upload failed, retrying", "position", position, "file", file.Path, "attempt", attempt, "max_attempts", UploadChunkMaxAttempts, "worker", wt.Name, "err", err)

		backoff := time.Duration(attempt) * 500 * time.Millisecond
		select {
		case <-ctx.Done():
			return uploadedChunkResult{}, ctx.Err()
		case <-time.After(backoff):
		}
	}

	return uploadedChunkResult{}, fmt.Errorf("uploading chunk %d: exhausted retries", position)
}

// Upload reads from reader chunk by chunk (streaming), uploads to Telegram in parallel,
// and records chunk metadata in the DB. Never holds the full file in memory.
func (m *StorageManager) Upload(ctx context.Context, file *domain.File, reader io.Reader, progress *UploadProgress) error {
	storage, err := m.storagesRepo.GetByID(ctx, file.StorageID)
	if err != nil {
		return fmt.Errorf("getting storage: %w", err)
	}

	slog.Info("starting upload", "file", file.Path, "storage", storage.Name, "chat", storage.Name)

	// Calculate total chunks from file size if known
	if progress != nil && progress.TotalBytes > 0 {
		total := int32(progress.TotalBytes / UploadChunkSize)
		if progress.TotalBytes%UploadChunkSize != 0 {
			total++
		}
		progress.TotalChunks = total
	}

	var mu sync.Mutex
	var results []uploadedChunkResult
	var failedChunks []failedChunkInfo

	// cleanupResultsSelective deletes the given chunk results from Telegram.
	cleanupResultsSelective := func(toClean []uploadedChunkResult) {
		if len(toClean) == 0 {
			return
		}
		slog.Warn("cleaning up chunks from telegram", "file", file.Path, "chunks", len(toClean))
		cleanupChunks := make([]domain.FileChunk, len(toClean))
		for i, r := range toClean {
			cleanupChunks[i] = domain.FileChunk{
				TelegramMessageID: r.TelegramMessageID,
			}
		}
		go func() {
			if err := m.DeleteFromTelegram(context.Background(), *storage, cleanupChunks, nil); err != nil {
				slog.Error("cleanup after failed upload returned error", "err", err)
			}
		}()
	}

	// cleanupAllResults deletes all uploaded chunks from Telegram.
	cleanupAllResults := func() {
		mu.Lock()
		uploaded := make([]uploadedChunkResult, len(results))
		copy(uploaded, results)
		mu.Unlock()
		cleanupResultsSelective(uploaded)
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(UploadChunkParallelism)

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

		uploadBuf := uploadChunkBufferPool.Get().([]byte)
		n, readErr := io.ReadFull(reader, uploadBuf)
		if n == 0 && readErr != nil {
			uploadChunkBufferPool.Put(uploadBuf)
			// Check if the read failed due to cancellation
			if ctx.Err() != nil {
				readCancelled = true
			}
			break
		}
		chunkData := uploadBuf[:n]
		pos := position
		plainHash := sha256.Sum256(chunkData)
		position++
		uploadBufRef := uploadBuf
		chunkDataRef := chunkData

		g.Go(func() error {
			defer uploadChunkBufferPool.Put(uploadBufRef)

			result, err := m.uploadChunkWithRetry(gctx, file, storage, pos, chunkDataRef, plainHash)
			if err != nil {
				// Context cancellation → propagate immediately
				if contextAborted(gctx, err) {
					return err
				}
				// Transient failure → save for second round instead of aborting
				dataCopy := make([]byte, len(chunkDataRef))
				copy(dataCopy, chunkDataRef)
				mu.Lock()
				failedChunks = append(failedChunks, failedChunkInfo{
					position:  pos,
					data:      dataCopy,
					plainHash: plainHash,
				})
				mu.Unlock()
				return nil
			}

			mu.Lock()
			results = append(results, result)
			mu.Unlock()

			if progress != nil {
				progress.UploadedChunks.Add(1)
				progress.UploadedBytes.Add(int64(len(chunkDataRef)))
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

	// If cancelled or errgroup propagated a context error, clean up
	if readCancelled || waitErr != nil {
		cleanupAllResults()
		if waitErr != nil {
			return waitErr
		}
		return ctx.Err()
	}

	// Second round: retry failed chunks sequentially (avoids rate limiting)
	if len(failedChunks) > 0 {
		slog.Warn("retrying failed chunks sequentially", "file", file.Path, "failed", len(failedChunks), "succeeded", len(results))
		for _, fc := range failedChunks {
			if ctx.Err() != nil {
				cleanupAllResults()
				return ctx.Err()
			}
			result, err := m.uploadChunkWithRetry(ctx, file, storage, fc.position, fc.data, fc.plainHash)
			if err != nil {
				cleanupAllResults()
				return fmt.Errorf("second-round upload chunk %d: %w", fc.position, err)
			}
			mu.Lock()
			results = append(results, result)
			mu.Unlock()

			if progress != nil {
				progress.UploadedChunks.Add(1)
				progress.UploadedBytes.Add(int64(len(fc.data)))
			}
		}
		slog.Info("second-round upload completed", "file", file.Path, "recovered", len(failedChunks))
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Position < results[j].Position
	})

	if progress != nil {
		progress.VerificationTotalChunks = int32(len(results))
		progress.VerifiedChunks.Store(0)
	}

	failedPositions, verifyErr := m.verifyUploadedChunks(ctx, file, *storage, results, progress)
	if verifyErr != nil {
		if len(failedPositions) > 0 {
			// Selective cleanup: only delete the chunks that failed verification
			failedSet := make(map[int16]struct{}, len(failedPositions))
			for _, pos := range failedPositions {
				failedSet[pos] = struct{}{}
			}
			var failedResults []uploadedChunkResult
			var goodResults []uploadedChunkResult
			for _, r := range results {
				if _, failed := failedSet[r.Position]; failed {
					failedResults = append(failedResults, r)
				} else {
					goodResults = append(goodResults, r)
				}
			}
			cleanupResultsSelective(failedResults)
			// Also clean up the good ones since we can't proceed with a partial upload
			cleanupResultsSelective(goodResults)
		} else {
			cleanupAllResults()
		}
		return verifyErr
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

	if err := m.filesRepo.CreateChunksAndMarkUploaded(ctx, file.ID, fileChunks); err != nil {
		cleanupAllResults()
		return fmt.Errorf("saving verified chunks: %w", err)
	}

	slog.Info("upload completed", "file", file.Path, "chunks", len(results), "storage", storage.Name, "chat", storage.Name)

	return nil
}

// verifyUploadedChunks verifies each uploaded chunk by downloading it and comparing
// its hash. Transient download failures are retried with backoff. Returns the
// positions of chunks that could not be verified so the caller can do selective cleanup.
func (m *StorageManager) verifyUploadedChunks(ctx context.Context, file *domain.File, storage domain.Storage, results []uploadedChunkResult, progress *UploadProgress) (failedPositions []int16, err error) {
	if len(results) == 0 {
		return nil, nil
	}

	parallelism := m.downloadParallelism(ctx, storage.ID, len(results))
	startedAt := time.Now()
	slog.Info("verifying upload", "file", file.Path, "chunks", len(results), "parallelism", parallelism, "storage", storage.Name)

	const verifyExtraAttempts = 2
	verifyBackoffs := []time.Duration{500 * time.Millisecond, 1 * time.Second}

	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(parallelism)

	for _, result := range results {
		result := result
		g.Go(func() error {
			chunk := domain.FileChunk{
				FileID:            file.ID,
				TelegramFileID:    result.TelegramFileID,
				TelegramMessageID: result.TelegramMessageID,
				Position:          result.Position,
			}

			var lastErr error
			for attempt := 0; attempt <= verifyExtraAttempts; attempt++ {
				if attempt > 0 {
					backoff := verifyBackoffs[attempt-1]
					select {
					case <-gctx.Done():
						return gctx.Err()
					case <-time.After(backoff):
					}
					slog.Warn("retrying chunk verification", "position", result.Position, "file", file.Path, "attempt", attempt+1)
				}

				chunkStartedAt := time.Now()
				data, err := m.downloadAndDecryptChunkCached(gctx, file.ID, storage, chunk, nil)
				if err != nil {
					if contextAborted(gctx, err) {
						return gctx.Err()
					}
					lastErr = fmt.Errorf("verifying chunk %d: %w", result.Position, err)
					slog.Warn("chunk verification download failed", "position", result.Position, "file", file.Path, "attempt", attempt+1, "elapsed", time.Since(chunkStartedAt).Round(time.Millisecond), "err", err)
					continue
				}

				if sha256.Sum256(data) != result.PlainHash {
					lastErr = fmt.Errorf("verifying chunk %d: uploaded chunk content mismatch after Telegram round-trip", result.Position)
					slog.Error("chunk verification content mismatch", "position", result.Position, "file", file.Path, "attempt", attempt+1, "elapsed", time.Since(chunkStartedAt).Round(time.Millisecond))
					// Hash mismatch is not transient — no point retrying
					break
				}

				// Verified successfully
				if progress != nil {
					progress.VerifiedChunks.Add(1)
				}
				slog.Info("chunk verified", "position", result.Position, "file", file.Path, "elapsed", time.Since(chunkStartedAt).Round(time.Millisecond))
				lastErr = nil
				break
			}

			if lastErr != nil {
				mu.Lock()
				failedPositions = append(failedPositions, result.Position)
				mu.Unlock()
			}
			return nil
		})
	}

	if waitErr := g.Wait(); waitErr != nil {
		slog.Error("upload verification aborted", "file", file.Path, "elapsed", time.Since(startedAt).Round(time.Millisecond), "err", waitErr)
		return nil, waitErr
	}

	if len(failedPositions) > 0 {
		slog.Error("upload verification completed with failures", "file", file.Path, "failed", len(failedPositions), "total", len(results), "elapsed", time.Since(startedAt).Round(time.Millisecond))
		return failedPositions, fmt.Errorf("verification failed for %d chunk(s)", len(failedPositions))
	}

	slog.Info("upload verification completed", "file", file.Path, "chunks", len(results), "elapsed", time.Since(startedAt).Round(time.Millisecond))
	return nil, nil
}
