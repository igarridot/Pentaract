package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
)

// erroringReader serves r and then fails with err instead of io.EOF, once
// ready is closed (nil ready means immediately).
type erroringReader struct {
	r     io.Reader
	err   error
	ready <-chan struct{}
}

func (e *erroringReader) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if err == io.EOF {
		if e.ready != nil {
			<-e.ready
		}
		return n, e.err
	}
	return n, err
}

// A read failure that is not a clean end of stream must fail the upload and
// remove the chunks already sent, never persist the partial file.
func TestUploadAbortsWhenSourceFailsMidStream(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New()
	tg := newFakeTelegram()
	// The source breaks once chunk 0 is on Telegram and being verified, so
	// the abort has something to clean up.
	chunkStored := make(chan struct{})
	var once sync.Once
	tg.serve = func(_ string, _ int16, stored []byte) []byte {
		once.Do(func() { close(chunkStored) })
		return stored
	}
	srv := httptest.NewServer(tg.handler(t))
	defer srv.Close()

	m, repo := newUploadTestManager(srv.URL, storageID, NewChunkCipher("secret"))

	sourceErr := errors.New("source connection reset")
	reader := &erroringReader{r: bytes.NewReader(make([]byte, UploadChunkSize)), err: sourceErr, ready: chunkStored}
	file := &domain.File{ID: fileID, Path: "docs/cut.bin", StorageID: storageID}

	err := m.Upload(context.Background(), file, reader, &UploadProgress{})
	if !errors.Is(err, sourceErr) {
		t.Fatalf("expected the source error, got %v", err)
	}
	if saved := repo.savedFor(fileID); saved != nil {
		t.Fatalf("truncated upload must not be persisted, got %+v", saved)
	}

	deadline := time.Now().Add(3 * time.Second)
	for tg.deletedCount() < 1 {
		if time.Now().After(deadline) {
			t.Fatalf("expected the uploaded chunk to be deleted from telegram, deleted=%d", tg.deletedCount())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// io.ErrUnexpectedEOF is the normal way a short last chunk ends and must keep
// working.
func TestUploadShortLastChunkStillPersists(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New()
	tg := newFakeTelegram()
	srv := httptest.NewServer(tg.handler(t))
	defer srv.Close()

	m, repo := newUploadTestManager(srv.URL, storageID, NewChunkCipher("secret"))
	file := &domain.File{ID: fileID, Path: "docs/tiny.bin", StorageID: storageID}

	if err := m.Upload(context.Background(), file, bytes.NewReader([]byte("tiny")), &UploadProgress{TotalBytes: 4}); err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if saved := repo.savedFor(fileID); len(saved) != 1 {
		t.Fatalf("expected one chunk record, got %+v", saved)
	}
}
