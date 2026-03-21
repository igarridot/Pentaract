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

	// cleanupResults deletes any chunks already uploaded to Telegram.
	cleanupResults := func() {
		mu.Lock()
		uploaded := make([]uploadedChunkResult, len(results))
		copy(uploaded, results)
		mu.Unlock()

		if len(uploaded) > 0 {
			slog.Warn("upload cancelled or failed, cleaning up chunks from telegram", "file", file.Path, "chunks", len(uploaded))
			cleanupChunks := make([]domain.FileChunk, len(uploaded))
			for i, r := range uploaded {
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
				return err
			}

			mu.Lock()
			results = append(results, result)
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

	sort.Slice(results, func(i, j int) bool {
		return results[i].Position < results[j].Position
	})

	if progress != nil {
		progress.VerificationTotalChunks = int32(len(results))
		progress.VerifiedChunks.Store(0)
	}

	if err := m.verifyUploadedChunks(ctx, file, *storage, results, progress); err != nil {
		cleanupResults()
		return err
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
		cleanupResults()
		return fmt.Errorf("saving verified chunks: %w", err)
	}

	slog.Info("upload completed", "file", file.Path, "chunks", len(results), "storage", storage.Name, "chat", storage.Name)

	return nil
}

func (m *StorageManager) verifyUploadedChunks(ctx context.Context, file *domain.File, storage domain.Storage, results []uploadedChunkResult, progress *UploadProgress) error {
	if len(results) == 0 {
		return nil
	}

	parallelism := m.downloadParallelism(ctx, storage.ID, len(results))
	startedAt := time.Now()
	slog.Info("verifying upload", "file", file.Path, "chunks", len(results), "parallelism", parallelism, "storage", storage.Name)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(parallelism)

	for _, result := range results {
		result := result
		g.Go(func() error {
			chunkStartedAt := time.Now()
			chunk := domain.FileChunk{
				FileID:            file.ID,
				TelegramFileID:    result.TelegramFileID,
				TelegramMessageID: result.TelegramMessageID,
				Position:          result.Position,
			}
			slog.Info("verifying chunk", "position", result.Position, "file", file.Path, "message_id", result.TelegramMessageID)

			data, err := m.downloadAndDecryptChunkCached(gctx, file.ID, storage, chunk, nil)
			if err != nil {
				slog.Error("chunk verification failed", "position", result.Position, "file", file.Path, "elapsed", time.Since(chunkStartedAt).Round(time.Millisecond), "err", err)
				return fmt.Errorf("verifying chunk %d: %w", result.Position, err)
			}

			if sha256.Sum256(data) != result.PlainHash {
				slog.Error("chunk verification content mismatch", "position", result.Position, "file", file.Path, "elapsed", time.Since(chunkStartedAt).Round(time.Millisecond))
				return fmt.Errorf("verifying chunk %d: uploaded chunk content mismatch after Telegram round-trip", result.Position)
			}

			if progress != nil {
				progress.VerifiedChunks.Add(1)
			}
			slog.Info("chunk verified", "position", result.Position, "file", file.Path, "elapsed", time.Since(chunkStartedAt).Round(time.Millisecond))

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		slog.Error("upload verification aborted", "file", file.Path, "elapsed", time.Since(startedAt).Round(time.Millisecond), "err", err)
		return err
	}

	slog.Info("upload verification completed", "file", file.Path, "chunks", len(results), "elapsed", time.Since(startedAt).Round(time.Millisecond))
	return nil
}
