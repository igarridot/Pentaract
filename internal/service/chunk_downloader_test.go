package service

import (
	"bytes"
	"context"
	"errors"
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

type fakeWorkersRepo struct {
	listFn func(ctx context.Context, sid uuid.UUID) ([]repository.WorkerToken, error)
}

func (f *fakeWorkersRepo) ListTokensByStorage(ctx context.Context, sid uuid.UUID) ([]repository.WorkerToken, error) {
	if f.listFn != nil {
		return f.listFn(ctx, sid)
	}
	return nil, nil
}

func TestDownloadChunkWorkerFallbackPrimaryFailsFallbackSucceeds(t *testing.T) {
	storageID := uuid.New()
	fakeWorkers := &fakeWorkersRepo{
		listFn: func(ctx context.Context, sid uuid.UUID) ([]repository.WorkerToken, error) {
			return []repository.WorkerToken{
				{Token: "TOKEN", Name: "w1"},
				{Token: "TOKEN2", Name: "w2"},
			}, nil
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/botTOKEN/getFile"):
			// Primary worker fails with getFile failure -> sentinel error
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: wrong file identifier"}`))
		case strings.Contains(r.URL.Path, "/botTOKEN2/getFile"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN2/path/chunk.bin"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("fallback-data"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m := &StorageManager{
		workersRepo: fakeWorkers,
		scheduler: NewWorkerScheduler(&fakeManagerSchedulerRepo{
			getTokenFn: func(ctx context.Context, sid uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
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
		t.Fatalf("expected fallback to succeed, got: %v", err)
	}
	if string(data) != "fallback-data" {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestDownloadChunkContextCancellationDuringFallback(t *testing.T) {
	storageID := uuid.New()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/getFile") {
			// All workers fail with getFile failure
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: wrong file identifier"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())

	fakeWorkers := &fakeWorkersRepo{
		listFn: func(ctx context.Context, sid uuid.UUID) ([]repository.WorkerToken, error) {
			// Cancel context when listing workers for fallback
			cancel()
			return []repository.WorkerToken{
				{Token: "TOKEN", Name: "w1"},
				{Token: "TOKEN2", Name: "w2"},
			}, nil
		},
	}

	m := &StorageManager{
		workersRepo: fakeWorkers,
		scheduler: NewWorkerScheduler(&fakeManagerSchedulerRepo{
			getTokenFn: func(ctx context.Context, sid uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
				return &repository.WorkerToken{Token: "TOKEN", Name: "w1"}, nil
			},
		}, 1),
		tgClient:    telegram.NewClient(srv.URL),
		chunkCipher: NewChunkCipher("secret"),
	}

	_, err := m.downloadChunk(ctx, domain.Storage{ID: storageID, ChatID: 123}, domain.FileChunk{
		TelegramFileID: "FILE_ID",
		Position:       0,
	})
	if err == nil {
		t.Fatalf("expected error after context cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestInferRangeChunkWindowByteCorrectness(t *testing.T) {
	tests := []struct {
		name            string
		start, end      int64
		totalSize       int64
		chunksCount     int
		wantStartIdx    int
		wantEndIdx      int
		wantOffset      int64
		wantOK          bool
	}{
		{
			name:         "single chunk always returns index 0",
			start:        5, end: 10,
			totalSize:   100,
			chunksCount: 1,
			wantStartIdx: 0, wantEndIdx: 0, wantOffset: 0, wantOK: true,
		},
		{
			name:         "zero chunks returns not-ok",
			start:        0, end: 10,
			totalSize:   100,
			chunksCount: 0,
			wantStartIdx: 0, wantEndIdx: 0, wantOffset: 0, wantOK: false,
		},
		{
			name:         "range in first chunk",
			start:        0, end: 100,
			totalSize:   int64(UploadChunkSize) + 100,
			chunksCount: 2,
			wantStartIdx: 0, wantEndIdx: 0, wantOffset: 0, wantOK: true,
		},
		{
			name:         "range in second chunk",
			start:        int64(UploadChunkSize) + 5,
			end:          int64(UploadChunkSize) + 10,
			totalSize:    int64(UploadChunkSize) + 100,
			chunksCount:  2,
			wantStartIdx: 1, wantEndIdx: 1, wantOffset: int64(UploadChunkSize), wantOK: true,
		},
		{
			name:         "range spanning both chunks",
			start:        int64(UploadChunkSize) - 10,
			end:          int64(UploadChunkSize) + 10,
			totalSize:    int64(UploadChunkSize) + 100,
			chunksCount:  2,
			wantStartIdx: 0, wantEndIdx: 1, wantOffset: 0, wantOK: true,
		},
		{
			name:         "three chunks range in middle",
			start:        int64(UploadChunkSize) + 5,
			end:          int64(UploadChunkSize) + 50,
			totalSize:    2*int64(UploadChunkSize) + 100,
			chunksCount:  3,
			wantStartIdx: 1, wantEndIdx: 1, wantOffset: int64(UploadChunkSize), wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startIdx, endIdx, offset, ok := inferRangeChunkWindow(tt.start, tt.end, tt.totalSize, tt.chunksCount)
			if ok != tt.wantOK {
				t.Fatalf("ok: got %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if startIdx != tt.wantStartIdx {
				t.Fatalf("startIdx: got %d, want %d", startIdx, tt.wantStartIdx)
			}
			if endIdx != tt.wantEndIdx {
				t.Fatalf("endIdx: got %d, want %d", endIdx, tt.wantEndIdx)
			}
			if offset != tt.wantOffset {
				t.Fatalf("offset: got %d, want %d", offset, tt.wantOffset)
			}
		})
	}
}

func TestDownloadAndDecryptChunkRoundTrip(t *testing.T) {
	storageID := uuid.New()
	fileID := uuid.New()
	plain := []byte("round-trip-test-data")
	cipher := NewChunkCipher("secret")
	enc, err := cipher.EncryptChunk(fileID, 0, plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

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
		scheduler: NewWorkerScheduler(&fakeManagerSchedulerRepo{}, 1),
		tgClient:    telegram.NewClient(srv.URL),
		chunkCipher: cipher,
	}

	data, err := m.downloadAndDecryptChunk(context.Background(), fileID, domain.Storage{ID: storageID, ChatID: 123}, domain.FileChunk{
		TelegramFileID: "FILE_ID",
		Position:       0,
	}, nil)
	if err != nil {
		t.Fatalf("downloadAndDecryptChunk failed: %v", err)
	}
	if !bytes.Equal(data, plain) {
		t.Fatalf("round-trip mismatch: got %q, want %q", string(data), string(plain))
	}
}

func TestDownloadAndDecryptChunkDecryptionFailure(t *testing.T) {
	storageID := uuid.New()
	fileID := uuid.New()
	// Create encrypted data with wrong fileID so decryption fails
	wrongFileID := uuid.New()
	cipher := NewChunkCipher("secret")
	enc, err := cipher.EncryptChunk(wrongFileID, 0, []byte("data"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

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
		scheduler:   NewWorkerScheduler(&fakeManagerSchedulerRepo{}, 1),
		tgClient:    telegram.NewClient(srv.URL),
		chunkCipher: cipher,
	}

	_, err = m.downloadAndDecryptChunk(context.Background(), fileID, domain.Storage{ID: storageID, ChatID: 123}, domain.FileChunk{
		TelegramFileID: "FILE_ID",
		Position:       0,
	}, nil)
	if err == nil {
		t.Fatalf("expected decryption failure")
	}
	if !errors.Is(err, domain.ErrDecryptionFailed) {
		t.Fatalf("expected ErrDecryptionFailed, got: %v", err)
	}
}

func TestDownloadChunkWithRetrySucceedsAfterTransientFailure(t *testing.T) {
	storageID := uuid.New()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			n := attempts.Add(1)
			if n < 3 {
				// First two attempts fail
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"ok":false,"description":"temporary error"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("recovered-data"))
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
		chunkCipher: NewChunkCipher("secret"),
	}

	data, err := m.downloadChunkWithRetry(context.Background(), domain.Storage{ID: storageID, ChatID: 123}, domain.FileChunk{
		TelegramFileID: "FILE_ID",
		Position:       0,
	}, nil)
	if err != nil {
		t.Fatalf("expected retry to succeed, got: %v", err)
	}
	if string(data) != "recovered-data" {
		t.Fatalf("unexpected content: %q", string(data))
	}
	if attempts.Load() < 3 {
		t.Fatalf("expected at least 3 attempts, got %d", attempts.Load())
	}
}

func TestDownloadChunkWithRetryExhaustsAttempts(t *testing.T) {
	storageID := uuid.New()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/getFile") {
			attempts.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"ok":false,"description":"persistent error"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
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
		chunkCipher: NewChunkCipher("secret"),
	}

	_, err := m.downloadChunkWithRetry(context.Background(), domain.Storage{ID: storageID, ChatID: 123}, domain.FileChunk{
		TelegramFileID: "FILE_ID",
		Position:       0,
	}, nil)
	if err == nil {
		t.Fatal("expected error after exhausting all attempts")
	}
	// Should have tried DownloadChunkMaxAttempts times (each attempt may also call worker fallback)
	if attempts.Load() < int32(DownloadChunkMaxAttempts) {
		t.Fatalf("expected at least %d attempts, got %d", DownloadChunkMaxAttempts, attempts.Load())
	}
}

func TestDownloadChunkWithRetryContextCancellation(t *testing.T) {
	storageID := uuid.New()
	ctx, cancel := context.WithCancel(context.Background())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/getFile") {
			cancel()
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"ok":false,"description":"error"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
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
		chunkCipher: NewChunkCipher("secret"),
	}

	_, err := m.downloadChunkWithRetry(ctx, domain.Storage{ID: storageID, ChatID: 123}, domain.FileChunk{
		TelegramFileID: "FILE_ID",
		Position:       0,
	}, nil)
	if err == nil {
		t.Fatal("expected error after context cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}
