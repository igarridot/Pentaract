package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/service"
)

type failingReadCloser struct {
	r   io.Reader
	err error
}

func (f *failingReadCloser) Read(p []byte) (int, error) {
	n, err := f.r.Read(p)
	if err == io.EOF {
		return n, f.err
	}
	return n, err
}

func (f *failingReadCloser) Close() error { return nil }

// A multipart body cut short surfaces as io.ErrUnexpectedEOF, which is also
// how a legitimate short last chunk looks to the uploader. The handler must
// turn source failures into domain.ErrUploadInterrupted so the upload aborts
// instead of persisting a truncated file.
func TestRunTrackedUploadWrapsSourceFailures(t *testing.T) {
	readErr := make(chan error, 1)
	h := newTestFilesHandler(&mockFilesService{
		uploadFn: func(ctx context.Context, userID, storageID uuid.UUID, path string, size int64, reader io.Reader, progress *service.UploadProgress, onConflict string) (*domain.File, bool, error) {
			_, err := io.ReadAll(reader)
			readErr <- err
			return nil, false, err
		},
	})
	tracker := &uploadTracker{progress: &service.UploadProgress{}}
	src := &failingReadCloser{r: bytes.NewReader([]byte("partial body")), err: io.ErrUnexpectedEOF}

	h.runTrackedUpload(context.Background(), tracker, uuid.New(), uuid.New(), "a/b.bin", 100, src, service.UploadConflictKeepBoth)

	err := <-readErr
	if !errors.Is(err, domain.ErrUploadInterrupted) || errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected ErrUploadInterrupted, got %v", err)
	}
	if !tracker.done || tracker.err == nil {
		t.Fatalf("tracker must record the failed upload, got done=%v err=%v", tracker.done, tracker.err)
	}
}
