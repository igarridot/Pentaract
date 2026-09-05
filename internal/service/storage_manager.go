package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/repository"
	"github.com/Dominux/Pentaract/internal/telegram"
)

// Repository subsets used by StorageManager, so tests can use small fakes.
type (
	workersLister interface {
		ListTokensByStorage(ctx context.Context, storageID uuid.UUID) ([]repository.WorkerToken, error)
	}
	chunksRepository interface {
		ListChunks(ctx context.Context, fileID uuid.UUID) ([]domain.FileChunk, error)
		UpdateChunkTelegramFileID(ctx context.Context, chunkID uuid.UUID, telegramFileID string) error
		CreateChunksAndMarkUploaded(ctx context.Context, fileID uuid.UUID, chunks []domain.FileChunk, size int64) error
	}
	storageGetter interface {
		GetByID(ctx context.Context, id uuid.UUID) (*domain.Storage, error)
	}
)

// chunkTransport is the Telegram plumbing every chunk operation needs.
type chunkTransport struct {
	workersRepo workersLister
	scheduler   *WorkerScheduler
	tgClient    *telegram.Client
	cipher      *ChunkCipher
}

// ChunkDeleter removes chunk messages from Telegram.
type ChunkDeleter struct {
	*chunkTransport
}

// ChunkDownloader fetches, decrypts and orders chunks for downloads, streams
// and byte-range requests, with a small cache for seek-heavy playback.
type ChunkDownloader struct {
	*chunkTransport
	filesRepo    chunksRepository
	storagesRepo storageGetter
	cache        *streamChunkCache
	loads        singleflight.Group
}

// ChunkUploader splits, encrypts, uploads and verifies chunks, persisting the
// records once the whole file is on Telegram.
type ChunkUploader struct {
	*chunkTransport
	filesRepo    chunksRepository
	storagesRepo storageGetter
	downloader   *ChunkDownloader // verification re-downloads what was uploaded
	deleter      *ChunkDeleter    // failed uploads are cleaned up
}

// StorageManager is the facade the services depend on: one object exposing
// upload, download and delete of chunked files.
type StorageManager struct {
	*ChunkUploader
	*ChunkDownloader
	*ChunkDeleter
}

// storageDeps groups the collaborators shared by the chunk services.
type storageDeps struct {
	filesRepo    chunksRepository
	storagesRepo storageGetter
	workersRepo  workersLister
	scheduler    *WorkerScheduler
	tgClient     *telegram.Client
	chunkCipher  *ChunkCipher
}

func NewStorageManager(
	filesRepo chunksRepository,
	storagesRepo storageGetter,
	workersRepo workersLister,
	scheduler *WorkerScheduler,
	tgClient *telegram.Client,
	chunkCipher *ChunkCipher,
) *StorageManager {
	return newStorageManager(storageDeps{
		filesRepo:    filesRepo,
		storagesRepo: storagesRepo,
		workersRepo:  workersRepo,
		scheduler:    scheduler,
		tgClient:     tgClient,
		chunkCipher:  chunkCipher,
	})
}

func newStorageManager(deps storageDeps) *StorageManager {
	transport := &chunkTransport{
		workersRepo: deps.workersRepo,
		scheduler:   deps.scheduler,
		tgClient:    deps.tgClient,
		cipher:      deps.chunkCipher,
	}
	deleter := &ChunkDeleter{chunkTransport: transport}
	downloader := &ChunkDownloader{
		chunkTransport: transport,
		filesRepo:      deps.filesRepo,
		storagesRepo:   deps.storagesRepo,
		cache:          newStreamChunkCache(defaultStreamChunkCacheMaxEntries, defaultStreamChunkCacheMaxBytes),
	}
	uploader := &ChunkUploader{
		chunkTransport: transport,
		filesRepo:      deps.filesRepo,
		storagesRepo:   deps.storagesRepo,
		downloader:     downloader,
		deleter:        deleter,
	}
	return &StorageManager{ChunkUploader: uploader, ChunkDownloader: downloader, ChunkDeleter: deleter}
}

func isGetFileFailure(err error) bool {
	return errors.Is(err, domain.ErrTelegramGetFileFailed) || errors.Is(err, domain.ErrTelegramResolveFailed)
}

func appendUniqueWorker(workers []repository.WorkerToken, worker repository.WorkerToken) []repository.WorkerToken {
	if worker.Token == "" {
		return workers
	}
	for _, existing := range workers {
		if existing.Token == worker.Token {
			return workers
		}
	}
	return append(workers, worker)
}

func validateEncryptedChunkSize(chunk []byte) error {
	if len(chunk) > MaxTelegramGetFileBytes {
		return fmt.Errorf("encrypted chunk size %d exceeds Telegram Bot API getFile limit %d", len(chunk), MaxTelegramGetFileBytes)
	}
	return nil
}

// contextAborted returns true only when the parent context has been cancelled
// or has expired. HTTP client timeouts wrap context.DeadlineExceeded internally,
// but those are transient errors that should be retried — so we only check the
// parent context, not the error chain.
func contextAborted(ctx context.Context) bool {
	return ctx.Err() != nil
}

// sleepBackoff waits attempt*500ms before the next retry, returning ctx.Err()
// if the context is cancelled while waiting. Used by the chunk upload and
// download retry loops.
func sleepBackoff(ctx context.Context, attempt int) error {
	backoff := time.Duration(attempt) * 500 * time.Millisecond
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(backoff):
		return nil
	}
}

// UploadProgress tracks chunk upload progress. All methods are safe to call
// on a nil receiver so callers that do not track progress can pass nil.
type UploadProgress struct {
	TotalChunks             int32
	UploadedChunks          atomic.Int32
	TotalBytes              int64
	UploadedBytes           atomic.Int64
	VerificationTotalChunks atomic.Int32 // atomic: updated concurrently by the verification pipeline
	VerifiedChunks          atomic.Int32
}

// expectedChunks derives TotalChunks from TotalBytes when the size is known.
func (p *UploadProgress) expectedChunks() {
	if p == nil || p.TotalBytes <= 0 {
		return
	}
	total := int32(p.TotalBytes / UploadChunkSize)
	if p.TotalBytes%UploadChunkSize != 0 {
		total++
	}
	p.TotalChunks = total
}

func (p *UploadProgress) setTotalChunks(total int32) {
	if p != nil {
		p.TotalChunks = total
	}
}

// chunkUploaded records one uploaded chunk that is now pending verification.
func (p *UploadProgress) chunkUploaded(bytes int) {
	if p == nil {
		return
	}
	p.UploadedChunks.Add(1)
	p.UploadedBytes.Add(int64(bytes))
	p.VerificationTotalChunks.Add(1)
}

func (p *UploadProgress) chunkVerified() {
	if p != nil {
		p.VerifiedChunks.Add(1)
	}
}

// DownloadProgress tracks chunk download progress. All methods are safe to
// call on a nil receiver.
type DownloadProgress struct {
	TotalChunks      int64
	DownloadedChunks atomic.Int64
	TotalBytes       int64
	DownloadedBytes  atomic.Int64
}

// setTotalsIfUnset fills the totals once; directory downloads pre-set them
// for the whole archive, so per-file downloads must not overwrite them.
func (p *DownloadProgress) setTotalsIfUnset(chunks, bytes int64) {
	if p == nil || p.TotalChunks != 0 || p.TotalBytes != 0 {
		return
	}
	p.TotalChunks = chunks
	p.TotalBytes = bytes
}

func (p *DownloadProgress) chunkDownloaded(bytes int64) {
	if p == nil {
		return
	}
	p.DownloadedChunks.Add(1)
	p.DownloadedBytes.Add(bytes)
}

// DeleteProgress tracks chunk deletion progress in Telegram. All methods are
// safe to call on a nil receiver.
type DeleteProgress struct {
	TotalChunks   int64
	DeletedChunks atomic.Int64
}

func (p *DeleteProgress) setTotalChunks(total int64) {
	if p != nil {
		p.TotalChunks = total
	}
}

func (p *DeleteProgress) chunkDeleted() {
	if p != nil {
		p.DeletedChunks.Add(1)
	}
}

type uploadedChunkResult struct {
	TelegramFileID    string
	TelegramMessageID int64
	Position          int16
	PlainHash         [sha256.Size]byte
}

// uploadParallelism calculates optimal upload concurrency based on
// available workers and rate limit, avoiding contention when few workers
// are configured.
func (u *ChunkUploader) uploadParallelism(ctx context.Context, storageID uuid.UUID) int {
	if u.workersRepo == nil || u.scheduler == nil {
		return UploadChunkParallelism
	}
	tokens, err := u.workersRepo.ListTokensByStorage(ctx, storageID)
	if err != nil || len(tokens) == 0 {
		return UploadChunkParallelism
	}
	// workers * rateLimit / 6 gives a reasonable concurrency:
	// 2 workers * 18 rpm / 6 = 6;  4 workers * 18 rpm / 6 = 12 (capped at 10)
	p := len(tokens) * u.scheduler.RateLimit() / 6
	if p < 2 {
		p = 2
	}
	if p > UploadChunkParallelism {
		p = UploadChunkParallelism
	}
	return p
}
