package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/telegram"
)

// fakeChunksRepo records what the upload persists.
type fakeChunksRepo struct {
	mu      sync.Mutex
	saved   map[uuid.UUID][]domain.FileChunk
	sizes   map[uuid.UUID]int64
	chunks  []domain.FileChunk
	updated map[uuid.UUID]string
	saveErr error
}

func newFakeChunksRepo() *fakeChunksRepo {
	return &fakeChunksRepo{saved: map[uuid.UUID][]domain.FileChunk{}, sizes: map[uuid.UUID]int64{}, updated: map[uuid.UUID]string{}}
}

func (f *fakeChunksRepo) ListChunks(context.Context, uuid.UUID) ([]domain.FileChunk, error) {
	return f.chunks, nil
}

func (f *fakeChunksRepo) UpdateChunkTelegramFileID(_ context.Context, chunkID uuid.UUID, telegramFileID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updated[chunkID] = telegramFileID
	return nil
}

func (f *fakeChunksRepo) CreateChunksAndMarkUploaded(_ context.Context, fileID uuid.UUID, chunks []domain.FileChunk, size int64) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saved[fileID] = chunks
	f.sizes[fileID] = size
	return nil
}

func (f *fakeChunksRepo) savedFor(fileID uuid.UUID) []domain.FileChunk {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.saved[fileID]
}

type fakeStorageGetter struct{ storage domain.Storage }

func (f fakeStorageGetter) GetByID(context.Context, uuid.UUID) (*domain.Storage, error) {
	s := f.storage
	return &s, nil
}

// fakeTelegram is an in-memory Telegram Bot API good enough for the upload
// pipeline: it stores sent documents and serves them back for verification.
type fakeTelegram struct {
	mu        sync.Mutex
	docs      map[string][]byte // file_id -> stored bytes
	positions map[string]int16  // file_id -> chunk position (from the filename)
	deleted   int
	nextID    int
	// serve lets a test replace what verification downloads for a chunk.
	serve func(fileID string, position int16, stored []byte) []byte
	// onSend runs before each sendDocument is answered.
	onSend func(attempt int) (status int, body string, handled bool)
}

func newFakeTelegram() *fakeTelegram {
	return &fakeTelegram{docs: map[string][]byte{}, positions: map[string]int16{}}
}

func (f *fakeTelegram) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/sendDocument"):
			f.mu.Lock()
			f.nextID++
			id := f.nextID
			onSend := f.onSend
			f.mu.Unlock()
			if onSend != nil {
				if status, body, handled := onSend(id); handled {
					w.WriteHeader(status)
					_, _ = io.WriteString(w, body)
					return
				}
			}
			if err := r.ParseMultipartForm(64 << 20); err != nil {
				t.Errorf("parse multipart: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			part, header, err := r.FormFile("document")
			if err != nil {
				t.Errorf("form file: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			data, _ := io.ReadAll(part)
			part.Close()
			pos, _ := strconv.Atoi(header.Filename[strings.LastIndex(header.Filename, "_")+1:])
			fileID := fmt.Sprintf("F%d", id)
			f.mu.Lock()
			f.docs[fileID] = data
			f.positions[fileID] = int16(pos)
			f.mu.Unlock()
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"ok":true,"result":{"message_id":%d,"document":{"file_id":%q}}}`, id, fileID)
		case strings.Contains(r.URL.Path, "/getFile"):
			fileID := r.URL.Query().Get("file_id")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"ok":true,"result":{"file_path":"p/%s"}}`, fileID)
		case strings.Contains(r.URL.Path, "/file/"):
			fileID := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			f.mu.Lock()
			stored, ok := f.docs[fileID]
			pos := f.positions[fileID]
			serve := f.serve
			f.mu.Unlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			if serve != nil {
				stored = serve(fileID, pos, stored)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(stored)
		case strings.Contains(r.URL.Path, "/deleteMessage"):
			f.mu.Lock()
			f.deleted++
			f.mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"ok":true}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func (f *fakeTelegram) deletedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deleted
}

func newUploadTestManager(srvURL string, storageID uuid.UUID, cipher *ChunkCipher) (*StorageManager, *fakeChunksRepo) {
	repo := newFakeChunksRepo()
	return newStorageManager(storageDeps{
		filesRepo:    repo,
		storagesRepo: fakeStorageGetter{storage: domain.Storage{ID: storageID, Name: "Main", ChatID: 123}},
		workersRepo:  &fakeWorkersRepo{},
		scheduler:    NewWorkerScheduler(&fakeManagerSchedulerRepo{}, 1),
		tgClient:     telegram.NewClient(srvURL),
		chunkCipher:  cipher,
	}), repo
}

func TestUploadStreamsChunksVerifiesAndPersistsInOrder(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New()
	cipher := NewChunkCipher("secret")
	tg := newFakeTelegram()
	srv := httptest.NewServer(tg.handler(t))
	defer srv.Close()

	m, repo := newUploadTestManager(srv.URL, storageID, cipher)

	// Two chunks: one full chunk plus a 5 byte tail.
	payload := bytes.Repeat([]byte("a"), UploadChunkSize)
	payload = append(payload, []byte("tail!")...)
	progress := &UploadProgress{TotalBytes: int64(len(payload))}
	file := &domain.File{ID: fileID, Path: "docs/big.bin", StorageID: storageID}

	if err := m.Upload(context.Background(), file, bytes.NewReader(payload), progress); err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	saved := repo.savedFor(fileID)
	if len(saved) != 2 || saved[0].Position != 0 || saved[1].Position != 1 || saved[0].FileID != fileID {
		t.Fatalf("expected two chunk records in position order, got %+v", saved)
	}
	if saved[0].TelegramFileID == "" || saved[0].TelegramMessageID == 0 {
		t.Fatalf("chunk records must carry telegram ids, got %+v", saved[0])
	}
	if got := repo.sizes[fileID]; got != int64(len(payload)) {
		t.Fatalf("expected the exact plaintext size %d to be stored, got %d", len(payload), got)
	}
	if progress.TotalChunks != 2 || progress.UploadedChunks.Load() != 2 || progress.VerifiedChunks.Load() != 2 {
		t.Fatalf("unexpected progress: total=%d uploaded=%d verified=%d", progress.TotalChunks, progress.UploadedChunks.Load(), progress.VerifiedChunks.Load())
	}
	if progress.UploadedBytes.Load() != int64(len(payload)) {
		t.Fatalf("unexpected uploaded bytes: %d", progress.UploadedBytes.Load())
	}

	// Telegram must hold the encrypted form of each chunk, decryptable per position.
	tg.mu.Lock()
	defer tg.mu.Unlock()
	if len(tg.docs) != 2 {
		t.Fatalf("expected 2 documents in telegram, got %d", len(tg.docs))
	}
	for id, stored := range tg.docs {
		plain, err := cipher.DecryptChunk(fileID, tg.positions[id], stored)
		if err != nil {
			t.Fatalf("stored chunk %s is not decryptable: %v", id, err)
		}
		if tg.positions[id] == 1 && string(plain) != "tail!" {
			t.Fatalf("unexpected tail chunk content: %q", plain)
		}
	}
	if tg.deleted != 0 {
		t.Fatalf("successful upload must not delete chunks, deleted=%d", tg.deleted)
	}
}

func TestUploadHashMismatchFailsAndCleansUpTelegram(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New()
	cipher := NewChunkCipher("secret")
	tg := newFakeTelegram()
	// Verification downloads a validly encrypted but different payload.
	tg.serve = func(_ string, position int16, _ []byte) []byte {
		enc, _, err := cipher.EncryptChunk(fileID, position, []byte("tampered"))
		if err != nil {
			t.Fatalf("encrypt tampered chunk: %v", err)
		}
		return append([]byte(nil), enc...)
	}
	srv := httptest.NewServer(tg.handler(t))
	defer srv.Close()

	m, repo := newUploadTestManager(srv.URL, storageID, cipher)

	file := &domain.File{ID: fileID, Path: "docs/small.bin", StorageID: storageID}
	err := m.Upload(context.Background(), file, strings.NewReader("hello"), &UploadProgress{TotalBytes: 5})
	if err == nil || !isHashMismatch(err) {
		t.Fatalf("expected hash mismatch failure, got %v", err)
	}
	if saved := repo.savedFor(fileID); saved != nil {
		t.Fatalf("no chunk records must be written on failure, got %+v", saved)
	}

	deadline := time.Now().Add(3 * time.Second)
	for tg.deletedCount() < 1 {
		if time.Now().After(deadline) {
			t.Fatalf("expected uploaded chunk to be deleted from telegram after failure")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestUploadCancelledContextAbortsWithoutPersisting(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New()
	cipher := NewChunkCipher("secret")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tg := newFakeTelegram()
	tg.onSend = func(int) (int, string, bool) {
		cancel()
		return http.StatusInternalServerError, `{"ok":false,"description":"boom"}`, true
	}
	srv := httptest.NewServer(tg.handler(t))
	defer srv.Close()

	m, repo := newUploadTestManager(srv.URL, storageID, cipher)

	file := &domain.File{ID: fileID, Path: "docs/cancel.bin", StorageID: storageID}
	err := m.Upload(ctx, file, strings.NewReader("hello"), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if saved := repo.savedFor(fileID); saved != nil {
		t.Fatalf("cancelled upload must not touch chunk records, got %+v", saved)
	}
}

func TestChunkVerifierSubmitDoesNotBlockOnceStopped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := newStorageManager(storageDeps{})
	v := m.startChunkVerifier(ctx, &domain.File{}, domain.Storage{}, nil, 0)

	// Trip the breaker and cancel: the consumer gives up while waiting for
	// the cooldown, which is exactly when producers used to block forever.
	for i := 0; i < VerifyCBFailureThreshold; i++ {
		v.cb.RecordFailure()
	}
	cancel()
	v.in <- uploadedChunkResult{} // consumed, then WaitIfTripped fails and the consumer exits
	select {
	case <-v.done:
	case <-time.After(time.Second):
		t.Fatal("consumer did not exit after cancellation")
	}

	done := make(chan error, 1)
	go func() { done <- v.submit(context.Background(), uploadedChunkResult{}) }()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled from stopped verifier, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("submit blocked on a verifier that is no longer consuming")
	}

	if err := v.stop(); err != nil {
		t.Fatalf("stop after cancellation should not report verification errors, got %v", err)
	}
	if err := v.stop(); err != nil {
		t.Fatalf("stop must be idempotent, got %v", err)
	}
}

func TestUploadPersistFailureCleansUpTelegram(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New()
	cipher := NewChunkCipher("secret")
	tg := newFakeTelegram()
	srv := httptest.NewServer(tg.handler(t))
	defer srv.Close()

	m, repo := newUploadTestManager(srv.URL, storageID, cipher)
	repo.saveErr = errors.New("db down")

	file := &domain.File{ID: fileID, Path: "docs/small.bin", StorageID: storageID}
	err := m.Upload(context.Background(), file, strings.NewReader("hello"), nil)
	if err == nil || !strings.Contains(err.Error(), "saving verified chunks") {
		t.Fatalf("expected persistence error, got %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for tg.deletedCount() < 1 {
		if time.Now().After(deadline) {
			t.Fatalf("expected uploaded chunk to be deleted from telegram after a persistence failure")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
