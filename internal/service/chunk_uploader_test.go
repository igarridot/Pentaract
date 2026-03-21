package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/repository"
	"github.com/Dominux/Pentaract/internal/telegram"
)

func TestUploadChunkWithRetrySuccess(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New()
	cipher := NewChunkCipher("secret")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/sendDocument") {
			w.WriteHeader(http.StatusOK)
			resp := map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": 42,
					"document": map[string]any{
						"file_id": "UPLOADED_FILE_ID",
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	m := &StorageManager{
		scheduler: NewWorkerScheduler(&fakeManagerSchedulerRepo{
			getTokenFn: func(ctx context.Context, sid uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
				return &repository.WorkerToken{Token: "TOKEN", Name: "w1"}, nil
			},
		}, 1),
		tgClient:    telegram.NewClient(srv.URL),
		chunkCipher: cipher,
	}

	file := &domain.File{ID: fileID, Path: "/test.txt", StorageID: storageID}
	storage := &domain.Storage{ID: storageID, ChatID: 123, Name: "test"}
	chunkData := []byte("test-chunk-data")

	result, err := m.uploadChunkWithRetry(context.Background(), file, storage, 0, chunkData, [32]byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TelegramFileID != "UPLOADED_FILE_ID" {
		t.Fatalf("expected UPLOADED_FILE_ID, got %q", result.TelegramFileID)
	}
	if result.TelegramMessageID != 42 {
		t.Fatalf("expected message_id 42, got %d", result.TelegramMessageID)
	}
	if result.Position != 0 {
		t.Fatalf("expected position 0, got %d", result.Position)
	}
}

func TestUploadChunkWithRetryRetriesOnTransientFailure(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New()
	cipher := NewChunkCipher("secret")

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/sendDocument") {
			n := attempts.Add(1)
			if n < 3 {
				// First two attempts fail with server error
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]any{"ok": false, "description": "Internal Server Error"})
				return
			}
			// Third attempt succeeds
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"message_id": 99,
					"document":   map[string]any{"file_id": "RETRY_FILE_ID"},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	m := &StorageManager{
		scheduler: NewWorkerScheduler(&fakeManagerSchedulerRepo{
			getTokenFn: func(ctx context.Context, sid uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
				return &repository.WorkerToken{Token: "TOKEN", Name: "w1"}, nil
			},
		}, 1),
		tgClient:    telegram.NewClient(srv.URL),
		chunkCipher: cipher,
	}

	file := &domain.File{ID: fileID, Path: "/retry.txt", StorageID: storageID}
	storage := &domain.Storage{ID: storageID, ChatID: 123, Name: "test"}

	result, err := m.uploadChunkWithRetry(context.Background(), file, storage, 0, []byte("data"), [32]byte{})
	if err != nil {
		t.Fatalf("expected retry to succeed, got: %v", err)
	}
	if result.TelegramFileID != "RETRY_FILE_ID" {
		t.Fatalf("expected RETRY_FILE_ID, got %q", result.TelegramFileID)
	}
	if int(attempts.Load()) != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts.Load())
	}
}

func TestUploadChunkWithRetryContextCancelled(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New()
	cipher := NewChunkCipher("secret")

	ctx, cancel := context.WithCancel(context.Background())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/sendDocument") {
			// Cancel context on first attempt
			cancel()
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "description": "error"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	m := &StorageManager{
		scheduler: NewWorkerScheduler(&fakeManagerSchedulerRepo{
			getTokenFn: func(ctx context.Context, sid uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
				return &repository.WorkerToken{Token: "TOKEN", Name: "w1"}, nil
			},
		}, 1),
		tgClient:    telegram.NewClient(srv.URL),
		chunkCipher: cipher,
	}

	file := &domain.File{ID: fileID, Path: "/cancel.txt", StorageID: storageID}
	storage := &domain.Storage{ID: storageID, ChatID: 123, Name: "test"}

	_, err := m.uploadChunkWithRetry(ctx, file, storage, 0, []byte("data"), [32]byte{})
	if err == nil {
		t.Fatalf("expected error after context cancellation")
	}
}

func TestShouldRetryChunkUpload(t *testing.T) {
	// nil error -> no retry
	if shouldRetryChunkUpload(context.Background(), nil) {
		t.Fatal("nil error should not retry")
	}

	// non-nil error with active context -> retry
	if !shouldRetryChunkUpload(context.Background(), fmt.Errorf("transient")) {
		t.Fatal("transient error should retry")
	}

	// cancelled context -> no retry
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if shouldRetryChunkUpload(ctx, context.Canceled) {
		t.Fatal("cancelled context should not retry")
	}
}

func TestVerifyUploadedChunksEmpty(t *testing.T) {
	m := &StorageManager{}
	failedPos, err := m.verifyUploadedChunks(context.Background(), &domain.File{}, domain.Storage{}, nil, nil)
	if err != nil {
		t.Fatalf("expected nil for empty results, got: %v", err)
	}
	if len(failedPos) != 0 {
		t.Fatalf("expected no failed positions, got: %v", failedPos)
	}
}

func TestVerifyUploadedChunksContentMismatch(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New()
	cipher := NewChunkCipher("secret")

	// Encrypt "original" data
	enc, _, err := cipher.EncryptChunk(fileID, 0, []byte("original"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"file_path": "path/chunk.bin"}})
		case strings.Contains(r.URL.Path, "/file/"):
			w.WriteHeader(http.StatusOK)
			w.Write(enc)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		workersRepo: &fakeWorkersRepo{},
		scheduler: NewWorkerScheduler(&fakeManagerSchedulerRepo{
			getTokenFn: func(ctx context.Context, sid uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
				return &repository.WorkerToken{Token: "TOKEN", Name: "w1"}, nil
			},
		}, 1),
		tgClient:    telegram.NewClient(srv.URL),
		chunkCipher: cipher,
	}

	// Provide a hash for "different-data" — verification should fail
	differentHash := sha256Hash([]byte("different-data"))

	results := []uploadedChunkResult{
		{
			TelegramFileID:    "FILE_ID",
			TelegramMessageID: 1,
			Position:          0,
			PlainHash:         differentHash,
		},
	}

	file := &domain.File{ID: fileID, StorageID: storageID}
	failedPos, err := m.verifyUploadedChunks(context.Background(), file, domain.Storage{ID: storageID, ChatID: 123}, results, nil)
	if err == nil {
		t.Fatal("expected content mismatch error")
	}
	if len(failedPos) != 1 || failedPos[0] != 0 {
		t.Fatalf("expected failed position [0], got: %v", failedPos)
	}
	if !strings.Contains(err.Error(), "failed for 1 chunk") {
		t.Fatalf("expected chunk failure error, got: %v", err)
	}
}

func TestVerifyUploadedChunksRetriesTransientDownloadFailure(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New()
	cipher := NewChunkCipher("secret")
	plainData := []byte("verify-retry-data")

	enc, _, err := cipher.EncryptChunk(fileID, 0, plainData)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	var downloadAttempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			n := downloadAttempts.Add(1)
			if n <= DownloadChunkMaxAttempts {
				// First DownloadChunkMaxAttempts attempts fail (covers downloadChunkWithRetry attempts)
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"ok":false,"description":"temporary error"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"file_path": "path/chunk.bin"}})
		case strings.Contains(r.URL.Path, "/file/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(enc)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		workersRepo: &fakeWorkersRepo{},
		scheduler: NewWorkerScheduler(&fakeManagerSchedulerRepo{
			getTokenFn: func(ctx context.Context, sid uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
				return &repository.WorkerToken{Token: "TOKEN", Name: "w1"}, nil
			},
		}, 1),
		tgClient:    telegram.NewClient(srv.URL),
		chunkCipher: cipher,
	}

	results := []uploadedChunkResult{
		{
			TelegramFileID:    "FILE_ID",
			TelegramMessageID: 1,
			Position:          0,
			PlainHash:         sha256Hash(plainData),
		},
	}

	file := &domain.File{ID: fileID, StorageID: storageID}
	failedPos, err := m.verifyUploadedChunks(context.Background(), file, domain.Storage{ID: storageID, ChatID: 123}, results, nil)
	if err != nil {
		t.Fatalf("expected verification to succeed after retry, got: %v", err)
	}
	if len(failedPos) != 0 {
		t.Fatalf("expected no failed positions, got: %v", failedPos)
	}
	// Should have needed more than 1 download attempt
	if downloadAttempts.Load() <= 1 {
		t.Fatalf("expected multiple download attempts, got %d", downloadAttempts.Load())
	}
}

func TestUploadChunkWithRetryBackoffRespectsContext(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New()
	cipher := NewChunkCipher("secret")

	ctx, cancel := context.WithCancel(context.Background())
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/sendDocument") {
			n := attempts.Add(1)
			if n == 1 {
				// First attempt fails, then cancel context during backoff
				go func() { cancel() }()
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]any{"ok": false, "description": "error"})
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"ok":     true,
				"result": map[string]any{"message_id": 1, "document": map[string]any{"file_id": "F"}},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	m := &StorageManager{
		scheduler: NewWorkerScheduler(&fakeManagerSchedulerRepo{
			getTokenFn: func(ctx context.Context, sid uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
				return &repository.WorkerToken{Token: "TOKEN", Name: "w1"}, nil
			},
		}, 1),
		tgClient:    telegram.NewClient(srv.URL),
		chunkCipher: cipher,
	}

	file := &domain.File{ID: fileID, Path: "/backoff.txt", StorageID: storageID}
	storage := &domain.Storage{ID: storageID, ChatID: 123, Name: "test"}

	_, err := m.uploadChunkWithRetry(ctx, file, storage, 0, []byte("data"), [32]byte{})
	if err == nil {
		t.Fatal("expected error after context cancellation during backoff")
	}
}

func TestUploadChunkBufferPoolReuse(t *testing.T) {
	// Get a buffer, verify its size, put it back, get another.
	buf := uploadChunkBufferPool.Get().([]byte)
	if len(buf) != UploadChunkSize {
		t.Fatalf("expected buffer size %d, got %d", UploadChunkSize, len(buf))
	}
	uploadChunkBufferPool.Put(buf)

	buf2 := uploadChunkBufferPool.Get().([]byte)
	if len(buf2) != UploadChunkSize {
		t.Fatalf("expected buffer size %d after reuse, got %d", UploadChunkSize, len(buf2))
	}
	uploadChunkBufferPool.Put(buf2)
}

func sha256Hash(data []byte) [32]byte {
	return sha256.Sum256(data)
}
