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
	// S5: use pooled encryption buffer
	encryptedChunkData, releaseEncBuf, err := m.chunkCipher.EncryptChunk(file.ID, position, chunkData)
	if err != nil {
		return uploadedChunkResult{}, fmt.Errorf("encrypting chunk %d: %w", position, err)
	}
	defer releaseEncBuf()

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

		// S6: propagate context to Telegram upload
		result, err := m.tgClient.Upload(ctx, wt.Token, storage.ChatID, encryptedChunkData, filename)
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

// verifySingleChunk downloads and hash-checks a single uploaded chunk.
// Returns nil on success, error on failure (after retries).
func (m *StorageManager) verifySingleChunk(ctx context.Context, file *domain.File, storage domain.Storage, result uploadedChunkResult) error {
	chunk := domain.FileChunk{
		FileID:            file.ID,
		TelegramFileID:    result.TelegramFileID,
		TelegramMessageID: result.TelegramMessageID,
		Position:          result.Position,
	}

	const verifyExtraAttempts = 2
	verifyBackoffs := []time.Duration{500 * time.Millisecond, 1 * time.Second}

	for attempt := 0; attempt <= verifyExtraAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(verifyBackoffs[attempt-1]):
			}
			slog.Warn("retrying chunk verification", "position", result.Position, "file", file.Path, "attempt", attempt+1)
		}

		chunkStartedAt := time.Now()
		data, err := m.downloadAndDecryptChunkCached(ctx, file.ID, storage, chunk)
		if err != nil {
			if contextAborted(ctx, err) {
				return ctx.Err()
			}
			slog.Warn("chunk verification download failed", "position", result.Position, "file", file.Path, "attempt", attempt+1, "elapsed", time.Since(chunkStartedAt).Round(time.Millisecond), "err", err)
			if attempt == verifyExtraAttempts {
				return fmt.Errorf("verifying chunk %d: %w", result.Position, err)
			}
			continue
		}

		if sha256.Sum256(data) != result.PlainHash {
			slog.Error("chunk verification content mismatch", "position", result.Position, "file", file.Path, "attempt", attempt+1, "elapsed", time.Since(chunkStartedAt).Round(time.Millisecond))
			// Hash mismatch is not transient — no point retrying
			return &chunkHashMismatchError{position: result.Position}
		}

		slog.Info("chunk verified", "position", result.Position, "file", file.Path, "elapsed", time.Since(chunkStartedAt).Round(time.Millisecond))
		return nil
	}

	return fmt.Errorf("verifying chunk %d: exhausted retries", result.Position)
}

// Upload reads from reader chunk by chunk (streaming), uploads to Telegram in parallel,
// and records chunk metadata in the DB. Never holds the full file in memory.
// S1: verification runs concurrently as chunks complete (pipeline).
// S7: upload parallelism adapts to available workers.
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
	var failedVerifyPositions []int16

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

	// S7: adaptive parallelism
	uploadPar := m.uploadParallelism(ctx, storage.ID)
	slog.Info("upload parallelism", "file", file.Path, "parallelism", uploadPar)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(uploadPar)

	// S1: pipeline verification — verify chunks as they finish uploading.
	// Uses PipelineVerifyParallelism (5) instead of VerifyChunkParallelism (10)
	// to avoid saturating the Telegram API when running alongside uploads.
	verifyCh := make(chan uploadedChunkResult, uploadPar)
	vg, vctx := errgroup.WithContext(ctx)
	vg.SetLimit(PipelineVerifyParallelism)

	cb := newVerifyCircuitBreaker()
	var verifyMu sync.Mutex
	var verifyRetryQueue []uploadedChunkResult
	verifyDone := make(chan struct{})
	go func() {
		defer close(verifyDone)
		for result := range verifyCh {
			result := result

			// Circuit breaker: if tripped, wait for the cooldown period
			// before dispatching more verifications.
			if err := cb.WaitIfTripped(vctx); err != nil {
				return
			}

			vg.Go(func() error {
				err := m.verifySingleChunk(vctx, file, *storage, result)
				if err != nil {
					if contextAborted(vctx, err) {
						return err
					}
					// Hash mismatch is permanent — fail the upload immediately.
					if isHashMismatch(err) {
						verifyMu.Lock()
						failedVerifyPositions = append(failedVerifyPositions, result.Position)
						verifyMu.Unlock()
						slog.Error("pipeline verification hash mismatch", "position", result.Position, "file", file.Path, "err", err)
						return fmt.Errorf("verification of chunk %d failed: %w", result.Position, err)
					}
					// Transient failure (download timeout / throttling) —
					// notify the circuit breaker and queue for retry after
					// cooldown instead of aborting the entire upload.
					cb.RecordFailure()
					verifyMu.Lock()
					verifyRetryQueue = append(verifyRetryQueue, result)
					verifyMu.Unlock()
					slog.Warn("pipeline verification transient failure, queued for retry",
						"position", result.Position, "file", file.Path,
						"consecutive_failures", cb.ConsecutiveFailures(), "err", err)
					return nil
				}
				cb.RecordSuccess()
				if progress != nil {
					progress.VerifiedChunks.Add(1)
				}
				return nil
			})
		}
	}()

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
				progress.VerificationTotalChunks.Add(1)
			}

			// S1: send for pipeline verification
			select {
			case verifyCh <- result:
			case <-gctx.Done():
				return gctx.Err()
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
		close(verifyCh)
		<-verifyDone
		vg.Wait()
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
				close(verifyCh)
				<-verifyDone
				vg.Wait()
				cleanupAllResults()
				return ctx.Err()
			}
			result, err := m.uploadChunkWithRetry(ctx, file, storage, fc.position, fc.data, fc.plainHash)
			if err != nil {
				close(verifyCh)
				<-verifyDone
				vg.Wait()
				cleanupAllResults()
				return fmt.Errorf("second-round upload chunk %d: %w", fc.position, err)
			}
			mu.Lock()
			results = append(results, result)
			mu.Unlock()

			if progress != nil {
				progress.UploadedChunks.Add(1)
				progress.UploadedBytes.Add(int64(len(fc.data)))
				progress.VerificationTotalChunks.Add(1)
			}

			// Send retried chunk for verification too
			verifyCh <- result
		}
		slog.Info("second-round upload completed", "file", file.Path, "recovered", len(failedChunks))
	}

	// Signal verification consumer to stop, then wait for all verifications
	close(verifyCh)
	<-verifyDone
	verifyWaitErr := vg.Wait()

	if verifyWaitErr != nil {
		cleanupAllResults()
		return verifyWaitErr
	}

	// Retry transiently failed verifications after a cooldown.
	// The circuit breaker queued these instead of aborting the upload.
	verifyMu.Lock()
	retryQueue := append([]uploadedChunkResult(nil), verifyRetryQueue...)
	verifyRetryQueue = nil
	verifyMu.Unlock()

	for retryRound := 0; len(retryQueue) > 0 && retryRound < VerifyCBMaxRetryRounds; retryRound++ {
		slog.Info("waiting before retrying failed verifications",
			"file", file.Path, "round", retryRound+1, "chunks", len(retryQueue),
			"cooldown", VerifyCBCooldownDuration)

		select {
		case <-ctx.Done():
			cleanupAllResults()
			return ctx.Err()
		case <-time.After(VerifyCBCooldownDuration):
		}

		var stillFailed []uploadedChunkResult
		for _, result := range retryQueue {
			if ctx.Err() != nil {
				cleanupAllResults()
				return ctx.Err()
			}
			err := m.verifySingleChunk(ctx, file, *storage, result)
			if err != nil {
				if contextAborted(ctx, err) {
					cleanupAllResults()
					return ctx.Err()
				}
				if isHashMismatch(err) {
					failedVerifyPositions = append(failedVerifyPositions, result.Position)
				} else {
					stillFailed = append(stillFailed, result)
				}
			} else {
				slog.Info("chunk verified on retry", "position", result.Position, "file", file.Path, "round", retryRound+1)
				if progress != nil {
					progress.VerifiedChunks.Add(1)
				}
			}
		}
		retryQueue = stillFailed

		if len(retryQueue) == 0 {
			slog.Info("all retried verifications succeeded", "file", file.Path, "round", retryRound+1)
		}
	}

	// Any chunks still failing after all retry rounds are permanent failures
	for _, result := range retryQueue {
		failedVerifyPositions = append(failedVerifyPositions, result.Position)
	}

	// Handle verification failures
	failedVPos := append([]int16(nil), failedVerifyPositions...)

	if len(failedVPos) > 0 {
		slog.Error("upload verification completed with failures", "file", file.Path, "failed", len(failedVPos), "total", len(results))
		failedSet := make(map[int16]struct{}, len(failedVPos))
		for _, pos := range failedVPos {
			failedSet[pos] = struct{}{}
		}
		var failedResults, goodResults []uploadedChunkResult
		for _, r := range results {
			if _, failed := failedSet[r.Position]; failed {
				failedResults = append(failedResults, r)
			} else {
				goodResults = append(goodResults, r)
			}
		}
		cleanupResultsSelective(failedResults)
		cleanupResultsSelective(goodResults)
		return fmt.Errorf("verification failed for %d chunk(s)", len(failedVPos))
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Position < results[j].Position
	})

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
// its hash. Retained for backward compatibility with tests.
func (m *StorageManager) verifyUploadedChunks(ctx context.Context, file *domain.File, storage domain.Storage, results []uploadedChunkResult, progress *UploadProgress) (failedPositions []int16, err error) {
	if len(results) == 0 {
		return nil, nil
	}

	parallelism := VerifyChunkParallelism
	startedAt := time.Now()
	slog.Info("verifying upload", "file", file.Path, "chunks", len(results), "parallelism", parallelism, "storage", storage.Name)

	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(parallelism)

	for _, result := range results {
		result := result
		g.Go(func() error {
			err := m.verifySingleChunk(gctx, file, storage, result)
			if err != nil {
				if contextAborted(gctx, err) {
					return err
				}
				mu.Lock()
				failedPositions = append(failedPositions, result.Position)
				mu.Unlock()
				return nil
			}
			if progress != nil {
				progress.VerifiedChunks.Add(1)
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
