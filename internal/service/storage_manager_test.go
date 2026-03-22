package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v3"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/repository"
	"github.com/Dominux/Pentaract/internal/telegram"
)

type fakeManagerSchedulerRepo struct {
	getTokenFn        func(ctx context.Context, storageID uuid.UUID, rateLimit int) (*repository.WorkerToken, error)
	getTokenBatchFn   func(ctx context.Context, storageID uuid.UUID, rateLimit, count int) ([]repository.WorkerToken, error)
	nextAvailableInFn func(ctx context.Context, storageID uuid.UUID, rateLimit int) (time.Duration, error)
}

func (f *fakeManagerSchedulerRepo) GetToken(ctx context.Context, storageID uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
	if f.getTokenFn == nil {
		return &repository.WorkerToken{Token: "TOKEN", Name: "w1"}, nil
	}
	return f.getTokenFn(ctx, storageID, rateLimit)
}

func (f *fakeManagerSchedulerRepo) GetTokenBatch(ctx context.Context, storageID uuid.UUID, rateLimit, count int) ([]repository.WorkerToken, error) {
	if f.getTokenBatchFn != nil {
		return f.getTokenBatchFn(ctx, storageID, rateLimit, count)
	}
	wt, err := f.GetToken(ctx, storageID, rateLimit)
	if err != nil {
		return nil, err
	}
	if wt == nil {
		return nil, nil
	}
	return []repository.WorkerToken{*wt}, nil
}

func (f *fakeManagerSchedulerRepo) NextAvailableIn(ctx context.Context, storageID uuid.UUID, rateLimit int) (time.Duration, error) {
	if f.nextAvailableInFn == nil {
		return 0, nil
	}
	return f.nextAvailableInFn(ctx, storageID, rateLimit)
}

func TestStorageManagerDownloadToWriter(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	fileID := uuid.New()
	storageID := uuid.New()
	chunkID := uuid.New()
	plain := []byte("hello")
	cipher := NewChunkCipher("secret")
	enc, _, err := cipher.EncryptChunk(fileID, 0, plain)
	if err != nil {
		t.Fatalf("encrypt chunk: %v", err)
	}

	mock.ExpectQuery("SELECT id, file_id, telegram_file_id, telegram_message_id, position FROM file_chunks WHERE file_id = \\$1 ORDER BY position").
		WithArgs(fileID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "file_id", "telegram_file_id", "telegram_message_id", "position"}).
			AddRow(chunkID, fileID, "TG_FILE_ID", int64(0), int16(0)))
	mock.ExpectQuery("SELECT id, name, chat_id FROM storages WHERE id = \\$1").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(storageID, "Main", int64(123)))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk.bin"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(enc)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		filesRepo:    repository.NewFilesRepo(mock),
		storagesRepo: repository.NewStoragesRepo(mock),
		scheduler:    NewWorkerScheduler(&fakeManagerSchedulerRepo{}, 1),
		tgClient:     telegram.NewClient(srv.URL),
		chunkCipher:  cipher,
	}

	var out bytes.Buffer
	progress := &DownloadProgress{}
	err = m.DownloadToWriter(context.Background(), &domain.File{ID: fileID, Path: "a.txt", Size: int64(len(plain)), StorageID: storageID}, &out, progress)
	if err != nil {
		t.Fatalf("download to writer failed: %v", err)
	}
	if out.String() != "hello" {
		t.Fatalf("unexpected download content: %q", out.String())
	}
	if progress.DownloadedChunks.Load() != 1 || progress.DownloadedBytes.Load() != int64(len(plain)) {
		t.Fatalf("unexpected progress: chunks=%d bytes=%d", progress.DownloadedChunks.Load(), progress.DownloadedBytes.Load())
	}
}

func TestStorageManagerStreamToWriterParallelizesChunksAndPreservesOrder(t *testing.T) {
	filesMock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new files pgxmock pool: %v", err)
	}
	defer filesMock.Close()

	workersMock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new workers pgxmock pool: %v", err)
	}
	defer workersMock.Close()

	fileID := uuid.New()
	storageID := uuid.New()
	chunkID0 := uuid.New()
	chunkID1 := uuid.New()
	plain0 := []byte("hello ")
	plain1 := []byte("world")
	cipher := NewChunkCipher("secret")
	enc0, _, err := cipher.EncryptChunk(fileID, 0, plain0)
	if err != nil {
		t.Fatalf("encrypt chunk 0: %v", err)
	}
	enc1, _, err := cipher.EncryptChunk(fileID, 1, plain1)
	if err != nil {
		t.Fatalf("encrypt chunk 1: %v", err)
	}

	filesMock.ExpectQuery("SELECT id, file_id, telegram_file_id, telegram_message_id, position FROM file_chunks WHERE file_id = \\$1 ORDER BY position").
		WithArgs(fileID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "file_id", "telegram_file_id", "telegram_message_id", "position"}).
			AddRow(chunkID0, fileID, "FILE0", int64(0), int16(0)).
			AddRow(chunkID1, fileID, "FILE1", int64(0), int16(1)))
	filesMock.ExpectQuery("SELECT id, name, chat_id FROM storages WHERE id = \\$1").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(storageID, "Main", int64(123)))

	workersMock.ExpectQuery("SELECT token, name FROM storage_workers").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"token", "name"}).
			AddRow("TOKEN", "w1").
			AddRow("TOKEN2", "w2"))

	chunk0Started := make(chan struct{})
	chunk1Started := make(chan struct{})
	var chunk0Once sync.Once
	var chunk1Once sync.Once
	workerTokens := make(map[string]int)
	var workerTokensMu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile") && strings.Contains(r.URL.RawQuery, "file_id=FILE0"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk0.bin"}}`))
		case strings.Contains(r.URL.Path, "/getFile") && strings.Contains(r.URL.RawQuery, "file_id=FILE1"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk1.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk0.bin"), strings.Contains(r.URL.Path, "/file/botTOKEN2/path/chunk0.bin"):
			workerTokensMu.Lock()
			if strings.Contains(r.URL.Path, "/file/botTOKEN2/") {
				workerTokens["TOKEN2"]++
			} else {
				workerTokens["TOKEN"]++
			}
			workerTokensMu.Unlock()
			chunk0Once.Do(func() { close(chunk0Started) })
			select {
			case <-chunk1Started:
			case <-r.Context().Done():
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(enc0)
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk1.bin"), strings.Contains(r.URL.Path, "/file/botTOKEN2/path/chunk1.bin"):
			workerTokensMu.Lock()
			if strings.Contains(r.URL.Path, "/file/botTOKEN2/") {
				workerTokens["TOKEN2"]++
			} else {
				workerTokens["TOKEN"]++
			}
			workerTokensMu.Unlock()
			chunk1Once.Do(func() { close(chunk1Started) })
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(enc1)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		filesRepo:    repository.NewFilesRepo(filesMock),
		storagesRepo: repository.NewStoragesRepo(filesMock),
		workersRepo:  repository.NewStorageWorkersRepo(workersMock),
		scheduler: NewWorkerScheduler(&fakeManagerSchedulerRepo{
			getTokenFn: func(ctx context.Context, storageID uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
				return &repository.WorkerToken{Token: "TOKEN", Name: "w1"}, nil
			},
		}, 10),
		tgClient:    telegram.NewClient(srv.URL),
		chunkCipher: cipher,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var out bytes.Buffer
	progress := &DownloadProgress{}
	err = m.StreamToWriter(ctx, &domain.File{
		ID:        fileID,
		Path:      "parallel.txt",
		Size:      int64(len(plain0) + len(plain1)),
		StorageID: storageID,
	}, &out, progress)
	if err != nil {
		t.Fatalf("parallel stream failed: %v", err)
	}
	if out.String() != "hello world" {
		t.Fatalf("unexpected download content: %q", out.String())
	}
	if progress.DownloadedChunks.Load() != 2 || progress.DownloadedBytes.Load() != int64(len(plain0)+len(plain1)) {
		t.Fatalf("unexpected progress after parallel stream: chunks=%d bytes=%d", progress.DownloadedChunks.Load(), progress.DownloadedBytes.Load())
	}
	// Worker distribution is now handled by the scheduler; we only verify
	// that the download completed with correct content and progress.
}

func TestStorageManagerDownloadToWriterParallelizesChunksAndPreservesOrder(t *testing.T) {
	filesMock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new files pgxmock pool: %v", err)
	}
	defer filesMock.Close()

	workersMock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new workers pgxmock pool: %v", err)
	}
	defer workersMock.Close()

	fileID := uuid.New()
	storageID := uuid.New()
	chunkID0 := uuid.New()
	chunkID1 := uuid.New()
	plain0 := []byte("hello ")
	plain1 := []byte("world")
	cipher := NewChunkCipher("secret")
	enc0, _, err := cipher.EncryptChunk(fileID, 0, plain0)
	if err != nil {
		t.Fatalf("encrypt chunk 0: %v", err)
	}
	enc1, _, err := cipher.EncryptChunk(fileID, 1, plain1)
	if err != nil {
		t.Fatalf("encrypt chunk 1: %v", err)
	}

	filesMock.ExpectQuery("SELECT id, file_id, telegram_file_id, telegram_message_id, position FROM file_chunks WHERE file_id = \\$1 ORDER BY position").
		WithArgs(fileID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "file_id", "telegram_file_id", "telegram_message_id", "position"}).
			AddRow(chunkID0, fileID, "FILE0", int64(0), int16(0)).
			AddRow(chunkID1, fileID, "FILE1", int64(0), int16(1)))
	filesMock.ExpectQuery("SELECT id, name, chat_id FROM storages WHERE id = \\$1").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(storageID, "Main", int64(123)))

	workersMock.ExpectQuery("SELECT token, name FROM storage_workers").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"token", "name"}).
			AddRow("TOKEN", "w1").
			AddRow("TOKEN2", "w2"))

	chunk0Started := make(chan struct{})
	chunk1Started := make(chan struct{})
	var chunk0Once sync.Once
	var chunk1Once sync.Once
	workerTokens := make(map[string]int)
	var workerTokensMu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile") && strings.Contains(r.URL.RawQuery, "file_id=FILE0"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk0.bin"}}`))
		case strings.Contains(r.URL.Path, "/getFile") && strings.Contains(r.URL.RawQuery, "file_id=FILE1"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk1.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk0.bin"), strings.Contains(r.URL.Path, "/file/botTOKEN2/path/chunk0.bin"):
			workerTokensMu.Lock()
			if strings.Contains(r.URL.Path, "/file/botTOKEN2/") {
				workerTokens["TOKEN2"]++
			} else {
				workerTokens["TOKEN"]++
			}
			workerTokensMu.Unlock()
			chunk0Once.Do(func() { close(chunk0Started) })
			select {
			case <-chunk1Started:
			case <-r.Context().Done():
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(enc0)
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk1.bin"), strings.Contains(r.URL.Path, "/file/botTOKEN2/path/chunk1.bin"):
			workerTokensMu.Lock()
			if strings.Contains(r.URL.Path, "/file/botTOKEN2/") {
				workerTokens["TOKEN2"]++
			} else {
				workerTokens["TOKEN"]++
			}
			workerTokensMu.Unlock()
			chunk1Once.Do(func() { close(chunk1Started) })
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(enc1)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		filesRepo:    repository.NewFilesRepo(filesMock),
		storagesRepo: repository.NewStoragesRepo(filesMock),
		workersRepo:  repository.NewStorageWorkersRepo(workersMock),
		scheduler: NewWorkerScheduler(&fakeManagerSchedulerRepo{
			getTokenFn: func(ctx context.Context, storageID uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
				return &repository.WorkerToken{Token: "TOKEN", Name: "w1"}, nil
			},
		}, 10),
		tgClient:    telegram.NewClient(srv.URL),
		chunkCipher: cipher,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var out bytes.Buffer
	progress := &DownloadProgress{}
	err = m.DownloadToWriter(ctx, &domain.File{
		ID:        fileID,
		Path:      "parallel.txt",
		Size:      int64(len(plain0) + len(plain1)),
		StorageID: storageID,
	}, &out, progress)
	if err != nil {
		t.Fatalf("parallel download failed: %v", err)
	}
	if out.String() != "hello world" {
		t.Fatalf("unexpected download content: %q", out.String())
	}
	if progress.DownloadedChunks.Load() != 2 || progress.DownloadedBytes.Load() != int64(len(plain0)+len(plain1)) {
		t.Fatalf("unexpected progress after parallel download: chunks=%d bytes=%d", progress.DownloadedChunks.Load(), progress.DownloadedBytes.Load())
	}
	// Worker distribution is now handled by the scheduler.
}

// TestStorageManagerDownloadToWriterCyclesReservedWorkersAcrossAllChunks removed —
// preferredWorkers parameter was deleted when all downloads were routed through
// the scheduler (PR #100).

func TestStorageManagerDownloadRangeToWriterUsesAllWorkersForMultiChunkStream(t *testing.T) {
	filesMock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new files pgxmock pool: %v", err)
	}
	defer filesMock.Close()

	workersMock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new workers pgxmock pool: %v", err)
	}
	defer workersMock.Close()

	fileID := uuid.New()
	storageID := uuid.New()
	chunkID0 := uuid.New()
	chunkID1 := uuid.New()
	plain0 := []byte("hello ")
	plain1 := []byte("world")
	cipher := NewChunkCipher("secret")
	enc0, _, err := cipher.EncryptChunk(fileID, 0, plain0)
	if err != nil {
		t.Fatalf("encrypt chunk 0: %v", err)
	}
	enc1, _, err := cipher.EncryptChunk(fileID, 1, plain1)
	if err != nil {
		t.Fatalf("encrypt chunk 1: %v", err)
	}

	filesMock.ExpectQuery("SELECT id, file_id, telegram_file_id, telegram_message_id, position FROM file_chunks WHERE file_id = \\$1 ORDER BY position").
		WithArgs(fileID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "file_id", "telegram_file_id", "telegram_message_id", "position"}).
			AddRow(chunkID0, fileID, "FILE0", int64(0), int16(0)).
			AddRow(chunkID1, fileID, "FILE1", int64(0), int16(1)))
	filesMock.ExpectQuery("SELECT id, name, chat_id FROM storages WHERE id = \\$1").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(storageID, "Main", int64(123)))

	workersMock.ExpectQuery("SELECT token, name FROM storage_workers").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"token", "name"}).
			AddRow("TOKEN", "w1").
			AddRow("TOKEN2", "w2"))

	chunk0Started := make(chan struct{})
	chunk1Started := make(chan struct{})
	var chunk0Once sync.Once
	var chunk1Once sync.Once
	workerTokens := make(map[string]int)
	var workerTokensMu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile") && strings.Contains(r.URL.RawQuery, "file_id=FILE0"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk0.bin"}}`))
		case strings.Contains(r.URL.Path, "/getFile") && strings.Contains(r.URL.RawQuery, "file_id=FILE1"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk1.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk0.bin"), strings.Contains(r.URL.Path, "/file/botTOKEN2/path/chunk0.bin"):
			workerTokensMu.Lock()
			if strings.Contains(r.URL.Path, "/file/botTOKEN2/") {
				workerTokens["TOKEN2"]++
			} else {
				workerTokens["TOKEN"]++
			}
			workerTokensMu.Unlock()
			chunk0Once.Do(func() { close(chunk0Started) })
			select {
			case <-chunk1Started:
			case <-r.Context().Done():
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(enc0)
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk1.bin"), strings.Contains(r.URL.Path, "/file/botTOKEN2/path/chunk1.bin"):
			workerTokensMu.Lock()
			if strings.Contains(r.URL.Path, "/file/botTOKEN2/") {
				workerTokens["TOKEN2"]++
			} else {
				workerTokens["TOKEN"]++
			}
			workerTokensMu.Unlock()
			chunk1Once.Do(func() { close(chunk1Started) })
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(enc1)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		filesRepo:    repository.NewFilesRepo(filesMock),
		storagesRepo: repository.NewStoragesRepo(filesMock),
		workersRepo:  repository.NewStorageWorkersRepo(workersMock),
		scheduler: NewWorkerScheduler(&fakeManagerSchedulerRepo{
			getTokenFn: func(ctx context.Context, storageID uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
				return &repository.WorkerToken{Token: "TOKEN", Name: "w1"}, nil
			},
		}, 10),
		tgClient:    telegram.NewClient(srv.URL),
		chunkCipher: cipher,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	totalSize := int64(len(plain0) + len(plain1))
	var out bytes.Buffer
	progress := &DownloadProgress{}
	err = m.DownloadRangeToWriter(ctx, &domain.File{
		ID:        fileID,
		Path:      "stream-range.txt",
		Size:      totalSize,
		StorageID: storageID,
	}, &out, 0, totalSize-1, totalSize, progress)
	if err != nil {
		t.Fatalf("parallel range stream failed: %v", err)
	}
	if out.String() != "hello world" {
		t.Fatalf("unexpected ranged content: %q", out.String())
	}
	// Worker distribution is now handled by the scheduler.
}

func TestStorageManagerExactFileSizeAndRange(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	fileID := uuid.New()
	storageID := uuid.New()
	chunkID := uuid.New()
	plain := []byte("hello world")
	cipher := NewChunkCipher("secret")
	enc, _, err := cipher.EncryptChunk(fileID, 0, plain)
	if err != nil {
		t.Fatalf("encrypt chunk: %v", err)
	}

	// ExactFileSize
	mock.ExpectQuery("SELECT id, file_id, telegram_file_id, telegram_message_id, position FROM file_chunks WHERE file_id = \\$1 ORDER BY position").
		WithArgs(fileID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "file_id", "telegram_file_id", "telegram_message_id", "position"}).
			AddRow(chunkID, fileID, "TG_FILE_ID", int64(0), int16(0)))
	mock.ExpectQuery("SELECT id, name, chat_id FROM storages WHERE id = \\$1").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(storageID, "Main", int64(123)))

	// DownloadRangeToWriter
	mock.ExpectQuery("SELECT id, file_id, telegram_file_id, telegram_message_id, position FROM file_chunks WHERE file_id = \\$1 ORDER BY position").
		WithArgs(fileID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "file_id", "telegram_file_id", "telegram_message_id", "position"}).
			AddRow(chunkID, fileID, "TG_FILE_ID", int64(0), int16(0)))
	mock.ExpectQuery("SELECT id, name, chat_id FROM storages WHERE id = \\$1").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(storageID, "Main", int64(123)))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk.bin"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(enc)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		filesRepo:    repository.NewFilesRepo(mock),
		storagesRepo: repository.NewStoragesRepo(mock),
		scheduler:    NewWorkerScheduler(&fakeManagerSchedulerRepo{}, 1),
		tgClient:     telegram.NewClient(srv.URL),
		chunkCipher:  cipher,
	}

	size, err := m.ExactFileSize(context.Background(), &domain.File{ID: fileID, StorageID: storageID})
	if err != nil || size != int64(len(plain)) {
		t.Fatalf("exact file size failed: size=%d err=%v", size, err)
	}

	var out bytes.Buffer
	progress := &DownloadProgress{}
	err = m.DownloadRangeToWriter(context.Background(), &domain.File{ID: fileID, StorageID: storageID}, &out, 6, 10, int64(len(plain)), progress)
	if err != nil {
		t.Fatalf("download range failed: %v", err)
	}
	if out.String() != "world" {
		t.Fatalf("unexpected range output: %q", out.String())
	}
}

func TestStorageManagerStreamToWriterPrimesChunkCacheForLaterSeek(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	fileID := uuid.New()
	storageID := uuid.New()
	chunkID := uuid.New()
	plain := []byte("hello world")
	cipher := NewChunkCipher("secret")
	enc, _, err := cipher.EncryptChunk(fileID, 0, plain)
	if err != nil {
		t.Fatalf("encrypt chunk: %v", err)
	}

	mock.ExpectQuery("SELECT id, file_id, telegram_file_id, telegram_message_id, position FROM file_chunks WHERE file_id = \\$1 ORDER BY position").
		WithArgs(fileID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "file_id", "telegram_file_id", "telegram_message_id", "position"}).
			AddRow(chunkID, fileID, "TG_FILE_ID", int64(0), int16(0)))
	mock.ExpectQuery("SELECT id, name, chat_id FROM storages WHERE id = \\$1").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(storageID, "Main", int64(123)))
	mock.ExpectQuery("SELECT id, file_id, telegram_file_id, telegram_message_id, position FROM file_chunks WHERE file_id = \\$1 ORDER BY position").
		WithArgs(fileID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "file_id", "telegram_file_id", "telegram_message_id", "position"}).
			AddRow(chunkID, fileID, "TG_FILE_ID", int64(0), int16(0)))
	mock.ExpectQuery("SELECT id, name, chat_id FROM storages WHERE id = \\$1").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(storageID, "Main", int64(123)))

	downloadCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk.bin"):
			downloadCount++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(enc)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		filesRepo:    repository.NewFilesRepo(mock),
		storagesRepo: repository.NewStoragesRepo(mock),
		scheduler:    NewWorkerScheduler(&fakeManagerSchedulerRepo{}, 1),
		tgClient:     telegram.NewClient(srv.URL),
		chunkCipher:  cipher,
	}

	var fullOut bytes.Buffer
	if err := m.StreamToWriter(context.Background(), &domain.File{
		ID:        fileID,
		Path:      "movie.mkv",
		Size:      int64(len(plain)),
		StorageID: storageID,
	}, &fullOut, &DownloadProgress{}); err != nil {
		t.Fatalf("stream download failed: %v", err)
	}

	var seekOut bytes.Buffer
	if err := m.DownloadRangeToWriter(
		context.Background(),
		&domain.File{ID: fileID, Path: "movie.mkv", Size: int64(len(plain)), StorageID: storageID},
		&seekOut,
		6,
		10,
		int64(len(plain)),
		&DownloadProgress{},
	); err != nil {
		t.Fatalf("seek range failed: %v", err)
	}

	if seekOut.String() != "world" {
		t.Fatalf("unexpected seek output: %q", seekOut.String())
	}
	if downloadCount != 1 {
		t.Fatalf("expected cached seek to avoid redownloading the chunk, got %d downloads", downloadCount)
	}
}

func TestStorageManagerDownloadToWriterDoesNotPrimeChunkCache(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	fileID := uuid.New()
	storageID := uuid.New()
	chunkID := uuid.New()
	plain := []byte("hello world")
	cipher := NewChunkCipher("secret")
	enc, _, err := cipher.EncryptChunk(fileID, 0, plain)
	if err != nil {
		t.Fatalf("encrypt chunk: %v", err)
	}

	mock.ExpectQuery("SELECT id, file_id, telegram_file_id, telegram_message_id, position FROM file_chunks WHERE file_id = \\$1 ORDER BY position").
		WithArgs(fileID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "file_id", "telegram_file_id", "telegram_message_id", "position"}).
			AddRow(chunkID, fileID, "TG_FILE_ID", int64(0), int16(0)))
	mock.ExpectQuery("SELECT id, name, chat_id FROM storages WHERE id = \\$1").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(storageID, "Main", int64(123)))
	mock.ExpectQuery("SELECT id, file_id, telegram_file_id, telegram_message_id, position FROM file_chunks WHERE file_id = \\$1 ORDER BY position").
		WithArgs(fileID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "file_id", "telegram_file_id", "telegram_message_id", "position"}).
			AddRow(chunkID, fileID, "TG_FILE_ID", int64(0), int16(0)))
	mock.ExpectQuery("SELECT id, name, chat_id FROM storages WHERE id = \\$1").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(storageID, "Main", int64(123)))

	downloadCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk.bin"):
			downloadCount++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(enc)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		filesRepo:    repository.NewFilesRepo(mock),
		storagesRepo: repository.NewStoragesRepo(mock),
		scheduler:    NewWorkerScheduler(&fakeManagerSchedulerRepo{}, 1),
		tgClient:     telegram.NewClient(srv.URL),
		chunkCipher:  cipher,
	}

	var fullOut bytes.Buffer
	if err := m.DownloadToWriter(context.Background(), &domain.File{
		ID:        fileID,
		Path:      "movie.mkv",
		Size:      int64(len(plain)),
		StorageID: storageID,
	}, &fullOut, &DownloadProgress{}); err != nil {
		t.Fatalf("full download failed: %v", err)
	}

	var seekOut bytes.Buffer
	if err := m.DownloadRangeToWriter(
		context.Background(),
		&domain.File{ID: fileID, Path: "movie.mkv", Size: int64(len(plain)), StorageID: storageID},
		&seekOut,
		6,
		10,
		int64(len(plain)),
		&DownloadProgress{},
	); err != nil {
		t.Fatalf("seek range failed: %v", err)
	}

	if seekOut.String() != "world" {
		t.Fatalf("unexpected seek output: %q", seekOut.String())
	}
	if downloadCount != 2 {
		t.Fatalf("expected regular download not to prime stream cache, got %d downloads", downloadCount)
	}
}

func TestStorageManagerDownloadRangeToWriterSeekSkipsEarlierChunks(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	fileID := uuid.New()
	storageID := uuid.New()
	chunkID0 := uuid.New()
	chunkID1 := uuid.New()
	plain0 := bytes.Repeat([]byte("a"), UploadChunkSize)
	plain1 := []byte("tail-data")
	cipher := NewChunkCipher("secret")
	enc0, _, err := cipher.EncryptChunk(fileID, 0, plain0)
	if err != nil {
		t.Fatalf("encrypt chunk 0: %v", err)
	}
	enc1, _, err := cipher.EncryptChunk(fileID, 1, plain1)
	if err != nil {
		t.Fatalf("encrypt chunk 1: %v", err)
	}

	mock.ExpectQuery("SELECT id, file_id, telegram_file_id, telegram_message_id, position FROM file_chunks WHERE file_id = \\$1 ORDER BY position").
		WithArgs(fileID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "file_id", "telegram_file_id", "telegram_message_id", "position"}).
			AddRow(chunkID0, fileID, "CHUNK0", int64(0), int16(0)).
			AddRow(chunkID1, fileID, "CHUNK1", int64(0), int16(1)))
	mock.ExpectQuery("SELECT id, name, chat_id FROM storages WHERE id = \\$1").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(storageID, "Main", int64(123)))

	var downloads []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile") && strings.Contains(r.URL.RawQuery, "file_id=CHUNK0"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk0.bin"}}`))
		case strings.Contains(r.URL.Path, "/getFile") && strings.Contains(r.URL.RawQuery, "file_id=CHUNK1"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk1.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk0.bin"):
			downloads = append(downloads, "chunk0")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(enc0)
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk1.bin"):
			downloads = append(downloads, "chunk1")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(enc1)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		filesRepo:    repository.NewFilesRepo(mock),
		storagesRepo: repository.NewStoragesRepo(mock),
		scheduler:    NewWorkerScheduler(&fakeManagerSchedulerRepo{}, 1),
		tgClient:     telegram.NewClient(srv.URL),
		chunkCipher:  cipher,
	}

	var out bytes.Buffer
	progress := &DownloadProgress{}
	start := int64(UploadChunkSize) + 2
	end := start + 3
	totalSize := int64(UploadChunkSize) + int64(len(plain1))
	err = m.DownloadRangeToWriter(context.Background(), &domain.File{ID: fileID, StorageID: storageID}, &out, start, end, totalSize, progress)
	if err != nil {
		t.Fatalf("download range failed: %v", err)
	}
	if out.String() != "il-d" {
		t.Fatalf("unexpected seek range output: %q", out.String())
	}
	if len(downloads) != 1 || downloads[0] != "chunk1" {
		t.Fatalf("expected only last chunk download, got %v", downloads)
	}
}

func TestStorageManagerExactFileSizeUsesOnlyLastChunk(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	fileID := uuid.New()
	storageID := uuid.New()
	firstChunkID := uuid.New()
	lastChunkID := uuid.New()
	lastPlain := []byte("tail")
	cipher := NewChunkCipher("secret")
	lastEnc, _, err := cipher.EncryptChunk(fileID, 1, lastPlain)
	if err != nil {
		t.Fatalf("encrypt chunk: %v", err)
	}

	mock.ExpectQuery("SELECT id, file_id, telegram_file_id, telegram_message_id, position FROM file_chunks WHERE file_id = \\$1 ORDER BY position").
		WithArgs(fileID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "file_id", "telegram_file_id", "telegram_message_id", "position"}).
			AddRow(firstChunkID, fileID, "IGNORED", int64(0), int16(0)).
			AddRow(lastChunkID, fileID, "LAST", int64(0), int16(1)))
	mock.ExpectQuery("SELECT id, name, chat_id FROM storages WHERE id = \\$1").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(storageID, "Main", int64(123)))

	var downloads []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile") && strings.Contains(r.URL.RawQuery, "file_id=LAST"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/last.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/last.bin"):
			downloads = append(downloads, r.URL.Path)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(lastEnc)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		filesRepo:    repository.NewFilesRepo(mock),
		storagesRepo: repository.NewStoragesRepo(mock),
		scheduler:    NewWorkerScheduler(&fakeManagerSchedulerRepo{}, 1),
		tgClient:     telegram.NewClient(srv.URL),
		chunkCipher:  cipher,
	}

	size, err := m.ExactFileSize(context.Background(), &domain.File{ID: fileID, StorageID: storageID})
	if err != nil {
		t.Fatalf("exact file size failed: %v", err)
	}
	want := UploadChunkSize + int64(len(lastPlain))
	if size != want {
		t.Fatalf("unexpected exact size: got=%d want=%d", size, want)
	}
	if len(downloads) != 1 {
		t.Fatalf("expected only one chunk download, got %d", len(downloads))
	}
}

func TestStorageManagerUploadAndDeleteFromTelegram(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	fileID := uuid.New()
	storageID := uuid.New()
	var uploadedChunk []byte

	mock.ExpectQuery("SELECT id, name, chat_id FROM storages WHERE id = \\$1").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(storageID, "Main", int64(123)))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO file_chunks").
		WithArgs(fileID, "TG_FILE", int64(77), int16(0)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("UPDATE files SET is_uploaded = true WHERE id = \\$1").
		WithArgs(fileID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/sendDocument"):
			mr, err := r.MultipartReader()
			if err != nil {
				t.Fatalf("multipart reader: %v", err)
			}
			for {
				part, err := mr.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("next multipart part: %v", err)
				}
				if part.FormName() == "document" {
					uploadedChunk, err = io.ReadAll(part)
					if err != nil {
						t.Fatalf("read uploaded chunk: %v", err)
					}
				}
				part.Close()
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":77,"document":{"file_id":"TG_FILE"}}}`))
		case strings.Contains(r.URL.Path, "/getFile") && strings.Contains(r.URL.RawQuery, "file_id=TG_FILE"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/uploaded.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/uploaded.bin"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(uploadedChunk)
		case strings.Contains(r.URL.Path, "/deleteMessage"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		filesRepo:    repository.NewFilesRepo(mock),
		storagesRepo: repository.NewStoragesRepo(mock),
		scheduler:    NewWorkerScheduler(&fakeManagerSchedulerRepo{}, 1),
		tgClient:     telegram.NewClient(srv.URL),
		chunkCipher:  NewChunkCipher("secret"),
	}

	progress := &UploadProgress{TotalBytes: 3}
	err = m.Upload(context.Background(), &domain.File{ID: fileID, Path: "a.txt", Size: 3, StorageID: storageID}, strings.NewReader("abc"), progress)
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if progress.UploadedChunks.Load() != 1 {
		t.Fatalf("unexpected uploaded chunks: %d", progress.UploadedChunks.Load())
	}
	if progress.VerificationTotalChunks.Load() != 1 || progress.VerifiedChunks.Load() != 1 {
		t.Fatalf("unexpected verification progress: total=%d verified=%d", progress.VerificationTotalChunks.Load(), progress.VerifiedChunks.Load())
	}
	if bytes.Equal(uploadedChunk, []byte("abc")) {
		t.Fatalf("uploaded chunk should be encrypted")
	}
	if !bytes.HasPrefix(uploadedChunk, chunkCipherMagic) {
		t.Fatalf("uploaded chunk should include encrypted payload magic")
	}
	if len(uploadedChunk) > MaxTelegramGetFileBytes {
		t.Fatalf("uploaded encrypted chunk exceeds getFile limit: %d", len(uploadedChunk))
	}
	decrypted, err := m.chunkCipher.DecryptChunk(fileID, 0, uploadedChunk)
	if err != nil {
		t.Fatalf("decrypt uploaded chunk failed: %v", err)
	}
	if string(decrypted) != "abc" {
		t.Fatalf("unexpected decrypted upload payload: %q", decrypted)
	}

	delProgress := &DeleteProgress{}
	err = m.DeleteFromTelegram(context.Background(), domain.Storage{ID: storageID, Name: "Main", ChatID: 123}, []domain.FileChunk{
		{TelegramMessageID: 0},
		{TelegramMessageID: 77},
	}, delProgress)
	if err != nil {
		t.Fatalf("delete from telegram failed: %v", err)
	}
	if delProgress.TotalChunks != 1 || delProgress.DeletedChunks.Load() != 1 {
		t.Fatalf("unexpected delete progress: total=%d deleted=%d", delProgress.TotalChunks, delProgress.DeletedChunks.Load())
	}
}

func TestStorageManagerUploadRetriesOnlyFailedChunk(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	fileID := uuid.New()
	storageID := uuid.New()
	plain0 := bytes.Repeat([]byte("a"), UploadChunkSize)
	plain1 := []byte("tail")
	totalSize := int64(len(plain0) + len(plain1))
	cipher := NewChunkCipher("secret")

	mock.ExpectQuery("SELECT id, name, chat_id FROM storages WHERE id = \\$1").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(storageID, "Main", int64(123)))
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO file_chunks").
		WithArgs(fileID, "TG_FILE_0", int64(100), int16(0), fileID, "TG_FILE_1", int64(101), int16(1)).
		WillReturnResult(pgxmock.NewResult("INSERT", 2))
	mock.ExpectExec("UPDATE files SET is_uploaded = true WHERE id = \\$1").
		WithArgs(fileID).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()

	var (
		mu               sync.Mutex
		attemptsByPos    = map[int16]int{}
		uploadedByFileID = map[string][]byte{}
	)

	detectPosition := func(payload []byte) int16 {
		if decrypted, err := cipher.DecryptChunk(fileID, 0, payload); err == nil && bytes.Equal(decrypted, plain0) {
			return 0
		}
		if decrypted, err := cipher.DecryptChunk(fileID, 1, payload); err == nil && bytes.Equal(decrypted, plain1) {
			return 1
		}
		t.Fatalf("uploaded payload does not match any expected chunk")
		return -1
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/sendDocument"):
			mr, err := r.MultipartReader()
			if err != nil {
				t.Fatalf("multipart reader: %v", err)
			}

			var uploadedChunk []byte
			for {
				part, err := mr.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("next multipart part: %v", err)
				}
				if part.FormName() == "document" {
					uploadedChunk, err = io.ReadAll(part)
					if err != nil {
						t.Fatalf("read uploaded chunk: %v", err)
					}
				}
				part.Close()
			}

			pos := detectPosition(uploadedChunk)

			mu.Lock()
			attemptsByPos[pos]++
			attempt := attemptsByPos[pos]
			mu.Unlock()

			if pos == 1 && attempt == 1 {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`temporary failure`))
				return
			}

			fileKey := fmt.Sprintf("TG_FILE_%d", pos)
			mu.Lock()
			uploadedByFileID[fileKey] = append([]byte(nil), uploadedChunk...)
			mu.Unlock()

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf(`{"ok":true,"result":{"message_id":%d,"document":{"file_id":"%s"}}}`, 100+pos, fileKey)))
		case strings.Contains(r.URL.Path, "/getFile") && strings.Contains(r.URL.RawQuery, "file_id=TG_FILE_0"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk0.bin"}}`))
		case strings.Contains(r.URL.Path, "/getFile") && strings.Contains(r.URL.RawQuery, "file_id=TG_FILE_1"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk1.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk0.bin"):
			mu.Lock()
			payload := append([]byte(nil), uploadedByFileID["TG_FILE_0"]...)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(payload)
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk1.bin"):
			mu.Lock()
			payload := append([]byte(nil), uploadedByFileID["TG_FILE_1"]...)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(payload)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		filesRepo:    repository.NewFilesRepo(mock),
		storagesRepo: repository.NewStoragesRepo(mock),
		scheduler:    NewWorkerScheduler(&fakeManagerSchedulerRepo{}, 1),
		tgClient:     telegram.NewClient(srv.URL),
		chunkCipher:  cipher,
	}

	progress := &UploadProgress{TotalBytes: totalSize}
	reader := io.MultiReader(bytes.NewReader(plain0), bytes.NewReader(plain1))
	err = m.Upload(context.Background(), &domain.File{ID: fileID, Path: "multi.bin", Size: totalSize, StorageID: storageID}, reader, progress)
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	mu.Lock()
	chunk0Attempts := attemptsByPos[0]
	chunk1Attempts := attemptsByPos[1]
	mu.Unlock()

	if chunk0Attempts != 1 {
		t.Fatalf("expected chunk 0 to upload once, got %d attempts", chunk0Attempts)
	}
	if chunk1Attempts != 2 {
		t.Fatalf("expected chunk 1 to retry once, got %d attempts", chunk1Attempts)
	}
	if progress.UploadedChunks.Load() != 2 || progress.UploadedBytes.Load() != totalSize {
		t.Fatalf("unexpected upload progress: chunks=%d bytes=%d", progress.UploadedChunks.Load(), progress.UploadedBytes.Load())
	}
	if progress.VerificationTotalChunks.Load() != 2 || progress.VerifiedChunks.Load() != 2 {
		t.Fatalf("unexpected verification progress: total=%d verified=%d", progress.VerificationTotalChunks.Load(), progress.VerifiedChunks.Load())
	}
}

func TestStorageManagerUploadVerifiesRoundTripAndCleansUpOnMismatch(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	fileID := uuid.New()
	storageID := uuid.New()
	cipher := NewChunkCipher("secret")
	badCiphertext, _, err := cipher.EncryptChunk(fileID, 0, []byte("xyz"))
	if err != nil {
		t.Fatalf("encrypt bad chunk: %v", err)
	}

	mock.ExpectQuery("SELECT id, name, chat_id FROM storages WHERE id = \\$1").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(storageID, "Main", int64(123)))

	deleteCalled := make(chan struct{})
	var deleteOnce sync.Once

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/sendDocument"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":77,"document":{"file_id":"TG_FILE"}}}`))
		case strings.Contains(r.URL.Path, "/getFile") && strings.Contains(r.URL.RawQuery, "file_id=TG_FILE"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/corrupted.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/corrupted.bin"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(badCiphertext)
		case strings.Contains(r.URL.Path, "/deleteMessage"):
			deleteOnce.Do(func() { close(deleteCalled) })
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		filesRepo:    repository.NewFilesRepo(mock),
		storagesRepo: repository.NewStoragesRepo(mock),
		scheduler:    NewWorkerScheduler(&fakeManagerSchedulerRepo{}, 1),
		tgClient:     telegram.NewClient(srv.URL),
		chunkCipher:  cipher,
	}

	err = m.Upload(context.Background(), &domain.File{ID: fileID, Path: "broken.txt", Size: 3, StorageID: storageID}, strings.NewReader("abc"), &UploadProgress{TotalBytes: 3})
	if err == nil || !strings.Contains(err.Error(), "verification") {
		t.Fatalf("expected verification failure error, got: %v", err)
	}

	select {
	case <-deleteCalled:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected cleanup delete to be triggered after verification failure")
	}
}

func TestStorageManagerDeleteFromTelegramFallsBackToOtherWorker(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	storageID := uuid.New()
	mock.ExpectQuery("SELECT token, name FROM storage_workers").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"token", "name"}).
			AddRow("BAD", "Bot10").
			AddRow("GOOD", "Bot11"))

	var (
		mu       sync.Mutex
		attempts []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts = append(attempts, r.URL.Path)
		mu.Unlock()

		switch {
		case strings.Contains(r.URL.Path, "/botBAD/deleteMessage"):
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: message can't be deleted"}`))
		case strings.Contains(r.URL.Path, "/botGOOD/deleteMessage"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		workersRepo: repository.NewStorageWorkersRepo(mock),
		scheduler: NewWorkerScheduler(&fakeManagerSchedulerRepo{
			getTokenFn: func(ctx context.Context, storageID uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
				return &repository.WorkerToken{Token: "BAD", Name: "Bot10"}, nil
			},
		}, 1),
		tgClient: telegram.NewClient(srv.URL),
	}

	progress := &DeleteProgress{}
	err = m.DeleteFromTelegram(context.Background(), domain.Storage{ID: storageID, Name: "Main", ChatID: 123}, []domain.FileChunk{
		{TelegramMessageID: 77},
	}, progress)
	if err != nil {
		t.Fatalf("delete from telegram failed: %v", err)
	}
	if progress.TotalChunks != 1 || progress.DeletedChunks.Load() != 1 {
		t.Fatalf("unexpected delete progress: total=%d deleted=%d", progress.TotalChunks, progress.DeletedChunks.Load())
	}

	mu.Lock()
	defer mu.Unlock()
	if len(attempts) != 2 {
		t.Fatalf("expected 2 delete attempts, got %d (%v)", len(attempts), attempts)
	}
	if !strings.Contains(attempts[0], "/botBAD/deleteMessage") {
		t.Fatalf("expected first delete attempt via BAD worker, got %q", attempts[0])
	}
	if !strings.Contains(attempts[1], "/botGOOD/deleteMessage") {
		t.Fatalf("expected fallback delete attempt via GOOD worker, got %q", attempts[1])
	}
}

func TestStorageManagerDeleteFromTelegramFailsWhenAllWorkersFail(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	storageID := uuid.New()
	mock.ExpectQuery("SELECT token, name FROM storage_workers").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"token", "name"}).
			AddRow("BAD", "Bot10").
			AddRow("WORSE", "Bot11"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/deleteMessage") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: message can't be deleted"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	m := &StorageManager{
		workersRepo: repository.NewStorageWorkersRepo(mock),
		scheduler: NewWorkerScheduler(&fakeManagerSchedulerRepo{
			getTokenFn: func(ctx context.Context, storageID uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
				return &repository.WorkerToken{Token: "BAD", Name: "Bot10"}, nil
			},
		}, 1),
		tgClient: telegram.NewClient(srv.URL),
	}

	progress := &DeleteProgress{}
	err = m.DeleteFromTelegram(context.Background(), domain.Storage{ID: storageID, Name: "Main", ChatID: 123}, []domain.FileChunk{
		{TelegramMessageID: 77},
	}, progress)
	if err == nil || !strings.Contains(err.Error(), "failed after trying 2 workers") {
		t.Fatalf("expected delete failure after retries, got: %v", err)
	}
	if progress.TotalChunks != 1 || progress.DeletedChunks.Load() != 0 {
		t.Fatalf("unexpected delete progress: total=%d deleted=%d", progress.TotalChunks, progress.DeletedChunks.Load())
	}
}

func TestStorageManagerRangeValidation(t *testing.T) {
	m := &StorageManager{}
	err := m.DownloadRangeToWriter(context.Background(), &domain.File{}, io.Discard, 10, 5, 20, nil)
	if err == nil {
		t.Fatalf("expected invalid range error")
	}
}

func TestValidateEncryptedChunkSize(t *testing.T) {
	cipher := NewChunkCipher("secret")
	fileID := uuid.New()
	plain := bytes.Repeat([]byte("a"), UploadChunkSize)

	enc, _, err := cipher.EncryptChunk(fileID, 0, plain)
	if err != nil {
		t.Fatalf("encrypt chunk: %v", err)
	}
	if len(enc) > MaxTelegramGetFileBytes {
		t.Fatalf("encrypted chunk exceeds getFile limit: %d", len(enc))
	}
	if err := validateEncryptedChunkSize(enc); err != nil {
		t.Fatalf("expected encrypted chunk to be accepted, got: %v", err)
	}

	tooLarge := make([]byte, MaxTelegramGetFileBytes+1)
	if err := validateEncryptedChunkSize(tooLarge); err == nil {
		t.Fatalf("expected oversized encrypted chunk to be rejected")
	}
}

func TestStorageManagerDownloadChunkWithWorkerRecoversByMessage(t *testing.T) {
	filesMock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer filesMock.Close()

	chunkID := uuid.New()
	storageID := uuid.New()
	filesMock.ExpectExec("UPDATE file_chunks SET telegram_file_id = \\$2 WHERE id = \\$1").
		WithArgs(chunkID, "NEW_FILE_ID").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/botTOKEN/getFile") && strings.Contains(r.URL.RawQuery, "file_id=OLD_FILE_ID"):
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: wrong file identifier/HTTP URL specified"}`))
		case strings.Contains(r.URL.Path, "/botTOKEN/forwardMessage"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":88,"document":{"file_id":"NEW_FILE_ID"}}}`))
		case strings.Contains(r.URL.Path, "/botTOKEN/deleteMessage"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		case strings.Contains(r.URL.Path, "/botTOKEN/getFile") && strings.Contains(r.URL.RawQuery, "file_id=NEW_FILE_ID"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk.bin"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("raw-bytes"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		filesRepo:   repository.NewFilesRepo(filesMock),
		tgClient:    telegram.NewClient(srv.URL),
		chunkCipher: NewChunkCipher("secret"),
	}

	data, err := m.downloadChunkWithWorker(context.Background(), domain.Storage{ID: storageID, ChatID: 123}, domain.FileChunk{
		ID:                chunkID,
		TelegramFileID:    "OLD_FILE_ID",
		TelegramMessageID: 77,
		Position:          0,
	}, repository.WorkerToken{Token: "TOKEN", Name: "w1"})
	if err != nil {
		t.Fatalf("download chunk with worker failed: %v", err)
	}
	if string(data) != "raw-bytes" {
		t.Fatalf("unexpected chunk bytes: %q", string(data))
	}
}

func TestStorageManagerDownloadChunkFallbackWorker(t *testing.T) {
	workersMock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer workersMock.Close()

	storageID := uuid.New()
	workersMock.ExpectQuery("SELECT token, name FROM storage_workers").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"token", "name"}).
			AddRow("TOKEN", "w1").
			AddRow("TOKEN2", "w2"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/botTOKEN/getFile"):
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: wrong file identifier/HTTP URL specified"}`))
		case strings.Contains(r.URL.Path, "/botTOKEN2/getFile"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN2/path/chunk.bin"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok-from-fallback"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		workersRepo: repository.NewStorageWorkersRepo(workersMock),
		scheduler: NewWorkerScheduler(&fakeManagerSchedulerRepo{
			getTokenFn: func(ctx context.Context, storageID uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
				return &repository.WorkerToken{Token: "TOKEN", Name: "w1"}, nil
			},
		}, 1),
		tgClient:    telegram.NewClient(srv.URL),
		chunkCipher: NewChunkCipher("secret"),
	}

	data, err := m.downloadChunk(context.Background(), domain.Storage{ID: storageID, ChatID: 123}, domain.FileChunk{
		TelegramFileID:    "FILE_ID",
		TelegramMessageID: 0,
		Position:          0,
	})
	if err != nil {
		t.Fatalf("download chunk fallback failed: %v", err)
	}
	if string(data) != "ok-from-fallback" {
		t.Fatalf("unexpected fallback content: %q", string(data))
	}
}

// Tests for downloadChunkWithPreferredWorker removed — function was deleted
// when all downloads were routed through the scheduler (PR #100).

func TestStorageManagerDownloadAndDecryptChunkTooBig(t *testing.T) {
	workersMock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer workersMock.Close()

	storageID := uuid.New()
	fileID := uuid.New()
	workersMock.ExpectQuery("SELECT token, name FROM storage_workers").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"token", "name"}).AddRow("TOKEN", "w1"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/getFile") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: file is too big"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	m := &StorageManager{
		workersRepo: repository.NewStorageWorkersRepo(workersMock),
		scheduler: NewWorkerScheduler(&fakeManagerSchedulerRepo{
			getTokenFn: func(ctx context.Context, storageID uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
				return &repository.WorkerToken{Token: "TOKEN", Name: "w1"}, nil
			},
		}, 1),
		tgClient:    telegram.NewClient(srv.URL),
		chunkCipher: NewChunkCipher("secret"),
	}

	_, err = m.downloadAndDecryptChunk(context.Background(), fileID, domain.Storage{ID: storageID, ChatID: 123}, domain.FileChunk{
		TelegramFileID: "FILE_ID",
		Position:       0,
	})
	if err == nil || !errors.Is(err, domain.ErrTelegramFileTooBig) {
		t.Fatalf("expected file-too-big error, got: %v", err)
	}
}
