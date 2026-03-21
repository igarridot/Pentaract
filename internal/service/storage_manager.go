package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/repository"
	"github.com/Dominux/Pentaract/internal/telegram"
)

// workersLister is the subset of StorageWorkersRepo used by StorageManager.
type workersLister interface {
	ListTokensByStorage(ctx context.Context, storageID uuid.UUID) ([]repository.WorkerToken, error)
}

type StorageManager struct {
	filesRepo          *repository.FilesRepo
	storagesRepo       *repository.StoragesRepo
	workersRepo        workersLister
	scheduler          *WorkerScheduler
	tgClient           *telegram.Client
	chunkCipher        *ChunkCipher
	streamChunkCache   *streamChunkCache
	streamChunkCacheMu sync.Mutex
	streamChunkLoads   singleflight.Group
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
	return errors.Is(err, domain.ErrTelegramGetFileFailed)
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
func contextAborted(ctx context.Context, err error) bool {
	return ctx != nil && ctx.Err() != nil
}

func contextAbortError(ctx context.Context, err error) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func (m *StorageManager) getStreamChunkCache() *streamChunkCache {
	m.streamChunkCacheMu.Lock()
	defer m.streamChunkCacheMu.Unlock()

	if m.streamChunkCache == nil {
		m.streamChunkCache = newStreamChunkCache(defaultStreamChunkCacheMaxEntries, defaultStreamChunkCacheMaxBytes)
	}

	return m.streamChunkCache
}

// UploadProgress tracks chunk upload progress.
type UploadProgress struct {
	TotalChunks             int32
	UploadedChunks          atomic.Int32
	TotalBytes              int64
	UploadedBytes           atomic.Int64
	VerificationTotalChunks int32
	VerifiedChunks          atomic.Int32
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

type uploadedChunkResult struct {
	TelegramFileID    string
	TelegramMessageID int64
	Position          int16
	PlainHash         [sha256.Size]byte
}
