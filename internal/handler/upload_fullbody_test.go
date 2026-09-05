package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	appjwt "github.com/Dominux/Pentaract/internal/jwt"
	"github.com/Dominux/Pentaract/internal/service"
)

// TestUploadOverRealServerDeliversWholeSmallBody uses a real net/http server:
// the handler answers 202 before the body is consumed, and net/http discards
// the unread body of small requests when the response starts unless the
// handler enables full duplex.
func TestUploadOverRealServerDeliversWholeSmallBody(t *testing.T) {
	payload := make([]byte, 100*1024)
	_, _ = rand.Read(payload)

	got := make(chan []byte, 1)
	h := newTestFilesHandler(&mockFilesService{
		uploadFn: func(ctx context.Context, userID, storageID uuid.UUID, path string, size int64, reader io.Reader, progress *service.UploadProgress, onConflict string) (*domain.File, bool, error) {
			b, _ := io.ReadAll(reader)
			got <- b
			return &domain.File{ID: uuid.New(), Path: path}, false, nil
		},
	})

	router := chi.NewRouter()
	router.Post("/storages/{storageID}/files/upload", func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), authUserKey, &appjwt.AuthUser{ID: uuid.New(), Email: "u@example.com"})
		h.Upload(w, r.WithContext(ctx))
	})
	srv := httptest.NewServer(router)
	defer srv.Close()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("path", "videos")
	_ = mw.WriteField("upload_id", "upload-small")
	_ = mw.WriteField("file_size", "102400")
	part, err := mw.CreateFormFile("file", "small.mp4")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = part.Write(payload)
	_ = mw.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/storages/"+uuid.New().String()+"/files/upload", bytes.NewReader(body.Bytes()))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	select {
	case b := <-got:
		if !bytes.Equal(b, payload) {
			t.Fatalf("upload service received %d bytes, want %d", len(b), len(payload))
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("upload service was not called")
	}
}
