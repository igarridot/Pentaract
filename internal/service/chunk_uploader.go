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
	return !contextAborted(ctx)
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

		if backoffErr := sleepBackoff(ctx, attempt); backoffErr != nil {
			return uploadedChunkResult{}, backoffErr
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
			if contextAborted(ctx) {
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

// Upload reads from reader chunk by chunk (streaming), uploads to Telegram in
// parallel and records chunk metadata in the DB. It never holds the full file
// in memory. Chunks are verified concurrently as they finish uploading, and
// the upload parallelism adapts to the available workers.
//
// Phases: parallel upload -> sequential retry of failed chunks -> drain the
// verifier -> retry transiently failed verifications -> persist. Any failure
// deletes the chunks already sent to Telegram.
func (m *StorageManager) Upload(ctx context.Context, file *domain.File, reader io.Reader, progress *UploadProgress) error {
	storage, err := m.storagesRepo.GetByID(ctx, file.StorageID)
	if err != nil {
		return fmt.Errorf("getting storage: %w", err)
	}

	slog.Info("starting upload", "file", file.Path, "storage", storage.Name, "chat", storage.Name)
	progress.expectedChunks()

	parallelism := m.uploadParallelism(ctx, storage.ID)
	slog.Info("upload parallelism", "file", file.Path, "parallelism", parallelism)

	run := &uploadRun{
		m:        m,
		ctx:      ctx,
		file:     file,
		storage:  storage,
		progress: progress,
		verifier: m.startChunkVerifier(ctx, file, *storage, progress, parallelism),
	}

	if err := run.uploadChunks(reader, parallelism); err != nil {
		return run.abort(err)
	}
	if err := run.retryFailedChunks(); err != nil {
		return run.abort(err)
	}
	if err := run.verifier.stop(); err != nil {
		return run.abort(err)
	}

	failedPositions, err := run.retryFailedVerifications()
	if err != nil {
		return run.abort(err)
	}
	if len(failedPositions) > 0 {
		slog.Error("upload verification completed with failures", "file", file.Path, "failed", len(failedPositions), "total", len(run.results))
		return run.abort(fmt.Errorf("verification failed for %d chunk(s)", len(failedPositions)))
	}

	return run.persist()
}

// uploadRun holds the state shared by the phases of a single Upload call.
type uploadRun struct {
	m        *StorageManager
	ctx      context.Context
	file     *domain.File
	storage  *domain.Storage
	progress *UploadProgress
	verifier *chunkVerifier

	mu           sync.Mutex
	results      []uploadedChunkResult
	failedChunks []failedChunkInfo
}

// uploadChunks streams the reader into chunks and uploads them in parallel.
// Chunks that fail transiently are collected for retryFailedChunks instead of
// failing the whole upload.
func (r *uploadRun) uploadChunks(reader io.Reader, parallelism int) error {
	g, gctx := errgroup.WithContext(r.ctx)
	g.SetLimit(parallelism)

	stopWatch := closeOnCancel(r.ctx, reader)
	defer stopWatch()

	var position int16
	for r.ctx.Err() == nil {
		buf := uploadChunkBufferPool.Get().([]byte)
		n, readErr := io.ReadFull(reader, buf)
		if n == 0 && readErr != nil {
			uploadChunkBufferPool.Put(buf)
			break
		}

		chunk := buf[:n]
		pos := position
		position++
		hash := sha256.Sum256(chunk)
		g.Go(func() error {
			defer uploadChunkBufferPool.Put(buf)
			return r.uploadChunk(gctx, pos, chunk, hash)
		})

		if readErr != nil {
			break
		}
	}

	r.progress.setTotalChunks(int32(position))

	if err := g.Wait(); err != nil {
		return err
	}
	return r.ctx.Err()
}

func (r *uploadRun) uploadChunk(ctx context.Context, position int16, data []byte, hash [sha256.Size]byte) error {
	result, err := r.m.uploadChunkWithRetry(ctx, r.file, r.storage, position, data, hash)
	if err != nil {
		if contextAborted(ctx) {
			return err
		}
		// Transient failure: keep an independent copy (the pool buffer is
		// reused) for the sequential second round instead of aborting.
		r.mu.Lock()
		r.failedChunks = append(r.failedChunks, failedChunkInfo{
			position:  position,
			data:      append([]byte(nil), data...),
			plainHash: hash,
		})
		r.mu.Unlock()
		return nil
	}

	r.recordResult(result, len(data))
	return r.verifier.submit(ctx, result)
}

func (r *uploadRun) recordResult(result uploadedChunkResult, bytes int) {
	r.mu.Lock()
	r.results = append(r.results, result)
	r.mu.Unlock()
	r.progress.chunkUploaded(bytes)
}

// retryFailedChunks re-uploads the chunks that failed during the parallel
// phase one at a time, which avoids tripping Telegram rate limits again.
func (r *uploadRun) retryFailedChunks() error {
	if len(r.failedChunks) == 0 {
		return nil
	}

	slog.Warn("retrying failed chunks sequentially", "file", r.file.Path, "failed", len(r.failedChunks), "succeeded", len(r.results))
	for _, fc := range r.failedChunks {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		result, err := r.m.uploadChunkWithRetry(r.ctx, r.file, r.storage, fc.position, fc.data, fc.plainHash)
		if err != nil {
			return fmt.Errorf("second-round upload chunk %d: %w", fc.position, err)
		}
		r.recordResult(result, len(fc.data))
		if err := r.verifier.submit(r.ctx, result); err != nil {
			return err
		}
	}
	slog.Info("second-round upload completed", "file", r.file.Path, "recovered", len(r.failedChunks))
	return nil
}

// retryFailedVerifications re-verifies the chunks the circuit breaker queued,
// waiting a cooldown before each round. It returns the positions that are
// still failing after all rounds (hash mismatches included).
func (r *uploadRun) retryFailedVerifications() ([]int16, error) {
	failed := r.verifier.failedPositions()
	retryQueue := r.verifier.takeRetryQueue()

	for round := 1; len(retryQueue) > 0 && round <= VerifyCBMaxRetryRounds; round++ {
		slog.Info("waiting before retrying failed verifications",
			"file", r.file.Path, "round", round, "chunks", len(retryQueue),
			"cooldown", VerifyCBCooldownDuration)

		select {
		case <-r.ctx.Done():
			return nil, r.ctx.Err()
		case <-time.After(VerifyCBCooldownDuration):
		}

		var stillFailed []uploadedChunkResult
		for _, result := range retryQueue {
			if err := r.ctx.Err(); err != nil {
				return nil, err
			}
			err := r.m.verifySingleChunk(r.ctx, r.file, *r.storage, result)
			switch {
			case err == nil:
				slog.Info("chunk verified on retry", "position", result.Position, "file", r.file.Path, "round", round)
				r.progress.chunkVerified()
			case contextAborted(r.ctx):
				return nil, r.ctx.Err()
			case isHashMismatch(err):
				failed = append(failed, result.Position)
			default:
				stillFailed = append(stillFailed, result)
			}
		}
		retryQueue = stillFailed

		if len(retryQueue) == 0 {
			slog.Info("all retried verifications succeeded", "file", r.file.Path, "round", round)
		}
	}

	for _, result := range retryQueue {
		failed = append(failed, result.Position)
	}
	return failed, nil
}

// persist writes the chunk records and marks the file as uploaded.
func (r *uploadRun) persist() error {
	sort.Slice(r.results, func(i, j int) bool {
		return r.results[i].Position < r.results[j].Position
	})

	fileChunks := make([]domain.FileChunk, len(r.results))
	for i, res := range r.results {
		fileChunks[i] = domain.FileChunk{
			FileID:            r.file.ID,
			TelegramFileID:    res.TelegramFileID,
			TelegramMessageID: res.TelegramMessageID,
			Position:          res.Position,
		}
	}

	if err := r.m.filesRepo.CreateChunksAndMarkUploaded(r.ctx, r.file.ID, fileChunks); err != nil {
		r.cleanupAll()
		return fmt.Errorf("saving verified chunks: %w", err)
	}

	slog.Info("upload completed", "file", r.file.Path, "chunks", len(r.results), "storage", r.storage.Name, "chat", r.storage.Name)
	return nil
}

// abort stops the verifier, removes every uploaded chunk from Telegram and
// returns err unchanged.
func (r *uploadRun) abort(err error) error {
	_ = r.verifier.stop()
	r.cleanupAll()
	return err
}

// cleanupAll deletes every chunk uploaded so far from Telegram in the
// background; the upload has already failed so the caller does not wait.
func (r *uploadRun) cleanupAll() {
	r.mu.Lock()
	uploaded := append([]uploadedChunkResult(nil), r.results...)
	r.mu.Unlock()
	if len(uploaded) == 0 {
		return
	}

	slog.Warn("cleaning up chunks from telegram", "file", r.file.Path, "chunks", len(uploaded))
	chunks := make([]domain.FileChunk, len(uploaded))
	for i, res := range uploaded {
		chunks[i] = domain.FileChunk{TelegramMessageID: res.TelegramMessageID}
	}
	storage := *r.storage
	go func() {
		if err := r.m.DeleteFromTelegram(context.Background(), storage, chunks, nil); err != nil {
			slog.Error("cleanup after failed upload returned error", "err", err)
		}
	}()
}

// closeOnCancel closes reader when ctx is cancelled so a blocked io.ReadFull
// unblocks. The returned function stops the watch.
func closeOnCancel(ctx context.Context, reader io.Reader) func() {
	closer, ok := reader.(io.Closer)
	if !ok {
		return func() {}
	}
	stopped := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			closer.Close()
		case <-stopped:
		}
	}()
	return func() { close(stopped) }
}

// chunkVerifier re-downloads uploaded chunks and checks their hash while the
// upload is still in flight. Transient failures trip a circuit breaker and are
// queued for a later retry round; a hash mismatch fails the verifier.
type chunkVerifier struct {
	m        *StorageManager
	ctx      context.Context
	file     *domain.File
	storage  domain.Storage
	progress *UploadProgress
	cb       *verifyCircuitBreaker
	group    *errgroup.Group
	in       chan uploadedChunkResult
	done     chan struct{}
	stopOnce sync.Once
	stopErr  error

	mu         sync.Mutex
	retryQueue []uploadedChunkResult
	failedPos  []int16
}

// startChunkVerifier runs the verification pipeline with at most
// PipelineVerifyParallelism concurrent verifications, so it does not saturate
// the Telegram API while uploads are still running.
func (m *StorageManager) startChunkVerifier(ctx context.Context, file *domain.File, storage domain.Storage, progress *UploadProgress, buffer int) *chunkVerifier {
	group, vctx := errgroup.WithContext(ctx)
	group.SetLimit(PipelineVerifyParallelism)
	v := &chunkVerifier{
		m:        m,
		ctx:      vctx,
		file:     file,
		storage:  storage,
		progress: progress,
		cb:       newVerifyCircuitBreaker(),
		group:    group,
		in:       make(chan uploadedChunkResult, buffer),
		done:     make(chan struct{}),
	}
	go v.consume()
	return v
}

func (v *chunkVerifier) consume() {
	defer close(v.done)
	for result := range v.in {
		// When the breaker is tripped, wait for the cooldown before
		// dispatching more verifications.
		if err := v.cb.WaitIfTripped(v.ctx); err != nil {
			return
		}
		v.group.Go(func() error { return v.verify(result) })
	}
}

// submit queues a chunk for verification. It also returns once the verifier
// itself has stopped (context cancelled or hash mismatch), so a caller never
// blocks on a pipeline that is no longer consuming.
func (v *chunkVerifier) submit(ctx context.Context, result uploadedChunkResult) error {
	select {
	case v.in <- result:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-v.ctx.Done():
		return v.ctx.Err()
	}
}

func (v *chunkVerifier) verify(result uploadedChunkResult) error {
	err := v.m.verifySingleChunk(v.ctx, v.file, v.storage, result)
	if err == nil {
		v.cb.RecordSuccess()
		v.progress.chunkVerified()
		return nil
	}
	if contextAborted(v.ctx) {
		return err
	}
	if isHashMismatch(err) {
		// Permanent: fail the upload immediately.
		v.mu.Lock()
		v.failedPos = append(v.failedPos, result.Position)
		v.mu.Unlock()
		slog.Error("pipeline verification hash mismatch", "position", result.Position, "file", v.file.Path, "err", err)
		return fmt.Errorf("verification of chunk %d failed: %w", result.Position, err)
	}

	// Transient (download timeout / throttling): notify the breaker and
	// queue for retry after the cooldown instead of aborting the upload.
	v.cb.RecordFailure()
	v.mu.Lock()
	v.retryQueue = append(v.retryQueue, result)
	v.mu.Unlock()
	slog.Warn("pipeline verification transient failure, queued for retry",
		"position", result.Position, "file", v.file.Path,
		"consecutive_failures", v.cb.ConsecutiveFailures(), "err", err)
	return nil
}

// stop closes the intake, waits for in-flight verifications and returns the
// first verification error. Safe to call more than once.
func (v *chunkVerifier) stop() error {
	v.stopOnce.Do(func() {
		close(v.in)
		<-v.done
		v.stopErr = v.group.Wait()
	})
	return v.stopErr
}

func (v *chunkVerifier) takeRetryQueue() []uploadedChunkResult {
	v.mu.Lock()
	defer v.mu.Unlock()
	queue := v.retryQueue
	v.retryQueue = nil
	return queue
}

func (v *chunkVerifier) failedPositions() []int16 {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]int16(nil), v.failedPos...)
}
