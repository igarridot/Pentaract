package service

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/telegram"
)

// blockingChunkServer serves one encrypted chunk, but holds every file
// download until release is closed. It reports how many downloads reached it.
type blockingChunkServer struct {
	srv       *httptest.Server
	release   chan struct{}
	started   chan struct{}
	startOnce sync.Once
	downloads atomic.Int32
}

func newBlockingChunkServer(encrypted []byte) *blockingChunkServer {
	s := &blockingChunkServer{release: make(chan struct{}), started: make(chan struct{})}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/"):
			s.downloads.Add(1)
			s.startOnce.Do(func() { close(s.started) })
			select {
			case <-s.release:
			case <-r.Context().Done():
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(encrypted)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return s
}

func newCachedChunkManager(t *testing.T, srvURL string, cipher *ChunkCipher) *StorageManager {
	t.Helper()
	return newStorageManager(storageDeps{
		filesRepo:    newFakeChunksRepo(),
		storagesRepo: fakeStorageGetter{},
		workersRepo:  &fakeWorkersRepo{},
		scheduler:    NewWorkerScheduler(&fakeManagerSchedulerRepo{}, 10),
		tgClient:     telegram.NewClient(srvURL),
		chunkCipher:  cipher,
	})
}

// A media player drops range requests all the time (seeks, probing the end
// of an MP4/MKV, reconnects). When the dropped request was the one actually
// downloading a chunk, the other requests waiting for that same chunk must
// not fail with the dropped request's cancellation.
func TestDownloadAndDecryptChunkCachedSurvivesLeaderCancellation(t *testing.T) {
	fileID := uuid.New()
	storage := domain.Storage{ID: uuid.New(), ChatID: 123}
	chunk := domain.FileChunk{ID: uuid.New(), FileID: fileID, TelegramFileID: "FILE0", Position: 0}
	plain := []byte("shared-chunk-bytes")
	cipher := NewChunkCipher("secret")
	enc, _, err := cipher.EncryptChunk(fileID, 0, plain)
	if err != nil {
		t.Fatalf("encrypt chunk: %v", err)
	}

	server := newBlockingChunkServer(enc)
	defer server.srv.Close()
	m := newCachedChunkManager(t, server.srv.URL, cipher)

	// Request A starts the download and becomes the singleflight leader.
	ctxA, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	errA := make(chan error, 1)
	go func() {
		_, err := m.downloadAndDecryptChunkCached(ctxA, fileID, storage, chunk)
		errA <- err
	}()
	select {
	case <-server.started:
	case <-time.After(2 * time.Second):
		t.Fatal("request A never reached the chunk download")
	}

	// Request B asks for the same chunk and joins A's in-flight load.
	ctxB, cancelB := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelB()
	type result struct {
		data []byte
		err  error
	}
	resB := make(chan result, 1)
	go func() {
		data, err := m.downloadAndDecryptChunkCached(ctxB, fileID, storage, chunk)
		resB <- result{data, err}
	}()
	// Give B time to attach to A's load before A goes away.
	time.Sleep(100 * time.Millisecond)

	// The player drops request A mid-download, then the download could proceed.
	cancelA()
	select {
	case err := <-errA:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("request A: expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request A did not return after cancellation")
	}
	close(server.release)

	select {
	case r := <-resB:
		if r.err != nil {
			t.Fatalf("request B must not inherit A's cancellation, got: %v", r.err)
		}
		if !bytes.Equal(r.data, plain) {
			t.Fatalf("request B got %q, want %q", r.data, plain)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request B never completed")
	}

	if got, ok := m.cache.get(streamChunkCacheKey{fileID: fileID, position: 0}); !ok || !bytes.Equal(got, plain) {
		t.Fatalf("chunk should be cached after B's load, got %q ok=%v", got, ok)
	}
}

// A caller whose own context is gone must still fail, not retry forever.
func TestDownloadAndDecryptChunkCachedHonoursOwnCancellation(t *testing.T) {
	fileID := uuid.New()
	storage := domain.Storage{ID: uuid.New(), ChatID: 123}
	chunk := domain.FileChunk{ID: uuid.New(), FileID: fileID, TelegramFileID: "FILE0", Position: 0}
	cipher := NewChunkCipher("secret")
	enc, _, err := cipher.EncryptChunk(fileID, 0, []byte("never-served"))
	if err != nil {
		t.Fatalf("encrypt chunk: %v", err)
	}

	server := newBlockingChunkServer(enc)
	defer server.srv.Close()
	defer close(server.release)
	m := newCachedChunkManager(t, server.srv.URL, cipher)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := m.downloadAndDecryptChunkCached(ctx, fileID, storage, chunk)
		done <- err
	}()
	select {
	case <-server.started:
	case <-time.After(2 * time.Second):
		t.Fatal("download never started")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled caller never returned")
	}
	if got := server.downloads.Load(); got != 1 {
		t.Fatalf("expected exactly one download attempt, got %d", got)
	}
}
