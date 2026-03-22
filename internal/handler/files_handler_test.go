package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	appjwt "github.com/Dominux/Pentaract/internal/jwt"
	"github.com/Dominux/Pentaract/internal/service"
)

type mockFilesService struct {
	moveFn                    func(ctx context.Context, userID, storageID uuid.UUID, oldPath, newPath string) error
	createFolderFn            func(ctx context.Context, userID, storageID uuid.UUID, path, folderName string) error
	uploadFn                  func(ctx context.Context, userID, storageID uuid.UUID, path string, size int64, reader io.Reader, progress *service.UploadProgress, onConflict string) (*domain.File, bool, error)
	deleteFn                  func(ctx context.Context, userID, storageID uuid.UUID, path string, progress *service.DeleteProgress, forceDelete bool) error
	workersStatusFn           func(storageID uuid.UUID) string
	getFileForDownloadFn      func(ctx context.Context, userID, storageID uuid.UUID, path string) (*domain.File, error)
	exactFileSizeFn           func(ctx context.Context, file *domain.File) (int64, error)
	downloadFileRangeToWriter func(ctx context.Context, file *domain.File, w io.Writer, start, end, totalSize int64, progress *service.DownloadProgress) error
	downloadFileToWriterFn    func(ctx context.Context, file *domain.File, w io.Writer, progress *service.DownloadProgress) error
	streamFileToWriterFn      func(ctx context.Context, file *domain.File, w io.Writer, progress *service.DownloadProgress) error
	downloadDirFn             func(ctx context.Context, userID, storageID uuid.UUID, dirPath string, w io.Writer, progress *service.DownloadProgress) (string, error)
	listDirFn                 func(ctx context.Context, userID, storageID uuid.UUID, path string) ([]domain.FSElement, error)
	searchFn                      func(ctx context.Context, userID, storageID uuid.UUID, basePath, searchPath string) ([]domain.FSElement, error)
	cleanupCancelledUploadFn      func(ctx context.Context, userID, storageID uuid.UUID, path string) error
}

func (m *mockFilesService) Move(ctx context.Context, userID, storageID uuid.UUID, oldPath, newPath string) error {
	if m.moveFn == nil {
		return nil
	}
	return m.moveFn(ctx, userID, storageID, oldPath, newPath)
}
func (m *mockFilesService) CreateFolder(ctx context.Context, userID, storageID uuid.UUID, path, folderName string) error {
	if m.createFolderFn == nil {
		return nil
	}
	return m.createFolderFn(ctx, userID, storageID, path, folderName)
}
func (m *mockFilesService) Upload(ctx context.Context, userID, storageID uuid.UUID, path string, size int64, reader io.Reader, progress *service.UploadProgress, onConflict string) (*domain.File, bool, error) {
	if m.uploadFn == nil {
		return &domain.File{}, false, nil
	}
	return m.uploadFn(ctx, userID, storageID, path, size, reader, progress, onConflict)
}
func (m *mockFilesService) Delete(ctx context.Context, userID, storageID uuid.UUID, path string, progress *service.DeleteProgress, forceDelete bool) error {
	if m.deleteFn == nil {
		return nil
	}
	return m.deleteFn(ctx, userID, storageID, path, progress, forceDelete)
}
func (m *mockFilesService) CleanupCancelledUpload(ctx context.Context, userID, storageID uuid.UUID, path string) error {
	if m.cleanupCancelledUploadFn == nil {
		return nil
	}
	return m.cleanupCancelledUploadFn(ctx, userID, storageID, path)
}
func (m *mockFilesService) WorkersStatus(storageID uuid.UUID) string {
	if m.workersStatusFn == nil {
		return "active"
	}
	return m.workersStatusFn(storageID)
}
func (m *mockFilesService) GetFileForDownload(ctx context.Context, userID, storageID uuid.UUID, path string) (*domain.File, error) {
	if m.getFileForDownloadFn == nil {
		return &domain.File{ID: uuid.New(), Path: path}, nil
	}
	return m.getFileForDownloadFn(ctx, userID, storageID, path)
}
func (m *mockFilesService) ExactFileSize(ctx context.Context, file *domain.File) (int64, error) {
	if m.exactFileSizeFn == nil {
		return file.Size, nil
	}
	return m.exactFileSizeFn(ctx, file)
}
func (m *mockFilesService) DownloadFileRangeToWriter(ctx context.Context, file *domain.File, w io.Writer, start, end, totalSize int64, progress *service.DownloadProgress) error {
	if m.downloadFileRangeToWriter == nil {
		return nil
	}
	return m.downloadFileRangeToWriter(ctx, file, w, start, end, totalSize, progress)
}
func (m *mockFilesService) DownloadFileToWriter(ctx context.Context, file *domain.File, w io.Writer, progress *service.DownloadProgress) error {
	if m.downloadFileToWriterFn == nil {
		_, _ = io.WriteString(w, "ok")
		return nil
	}
	return m.downloadFileToWriterFn(ctx, file, w, progress)
}
func (m *mockFilesService) StreamFileToWriter(ctx context.Context, file *domain.File, w io.Writer, progress *service.DownloadProgress) error {
	if m.streamFileToWriterFn == nil {
		_, _ = io.WriteString(w, "ok")
		return nil
	}
	return m.streamFileToWriterFn(ctx, file, w, progress)
}
func (m *mockFilesService) DownloadDir(ctx context.Context, userID, storageID uuid.UUID, dirPath string, w io.Writer, progress *service.DownloadProgress) (string, error) {
	if m.downloadDirFn == nil {
		return "files", nil
	}
	return m.downloadDirFn(ctx, userID, storageID, dirPath, w, progress)
}
func (m *mockFilesService) ListDir(ctx context.Context, userID, storageID uuid.UUID, path string) ([]domain.FSElement, error) {
	if m.listDirFn == nil {
		return nil, nil
	}
	return m.listDirFn(ctx, userID, storageID, path)
}
func (m *mockFilesService) Search(ctx context.Context, userID, storageID uuid.UUID, basePath, searchPath string) ([]domain.FSElement, error) {
	if m.searchFn == nil {
		return nil, nil
	}
	return m.searchFn(ctx, userID, storageID, basePath, searchPath)
}

func makeFilesReq(method, target, body, storageID, wildcard string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	rctx := chi.NewRouteContext()
	if storageID != "" {
		rctx.URLParams.Add("storageID", storageID)
	}
	if wildcard != "" {
		rctx.URLParams.Add("*", wildcard)
	}
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(context.WithValue(req.Context(), authUserKey, &appjwt.AuthUser{ID: uuid.New(), Email: "u@example.com"}))
	return req
}

func TestNewFilesHandler(t *testing.T) {
	h := NewFilesHandler(&mockFilesService{})
	if h == nil || h.svc == nil || h.uploads == nil || h.downloads == nil {
		t.Fatalf("expected initialized files handler")
	}
}

func TestFilesHandlerTreeAndSearchNilBecomeEmpty(t *testing.T) {
	h := NewFilesHandler(&mockFilesService{})
	storageID := uuid.New().String()

	w := httptest.NewRecorder()
	h.Tree(w, makeFilesReq(http.MethodGet, "/", "", storageID, "docs"))
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("tree expected [] and 200, got %d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.Search(w, makeFilesReq(http.MethodGet, "/?search_path=abc", "", storageID, "docs"))
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("search expected [] and 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestFilesHandlerDeleteFileValidationAndSuccess(t *testing.T) {
	var gotPath string
	var gotForce bool
	h := NewFilesHandler(&mockFilesService{
		deleteFn: func(ctx context.Context, userID, storageID uuid.UUID, path string, progress *service.DeleteProgress, forceDelete bool) error {
			gotPath = path
			gotForce = forceDelete
			return nil
		},
	})
	storageID := uuid.New().String()

	w := httptest.NewRecorder()
	h.DeleteFile(w, makeFilesReq(http.MethodDelete, "/", "", storageID, ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty path, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.DeleteFile(w, makeFilesReq(http.MethodDelete, "/?force_delete=nope", "", storageID, "a.txt"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid force_delete, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.DeleteFile(w, makeFilesReq(http.MethodDelete, "/?force_delete=true&delete_id=del-1", "", storageID, "folder/a.txt/"))
	if w.Code != http.StatusNoContent || gotPath != "folder/a.txt" || !gotForce {
		t.Fatalf("unexpected delete result: code=%d path=%s force=%v", w.Code, gotPath, gotForce)
	}
}

func TestFilesHandlerDownloadAttachment(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New().String()
	h := NewFilesHandler(&mockFilesService{
		getFileForDownloadFn: func(ctx context.Context, userID, storageID uuid.UUID, path string) (*domain.File, error) {
			return &domain.File{ID: fileID, Path: "folder/a.txt", Size: 3}, nil
		},
		downloadFileToWriterFn: func(ctx context.Context, file *domain.File, w io.Writer, progress *service.DownloadProgress) error {
			_, _ = io.WriteString(w, "abc")
			return nil
		},
	})

	w := httptest.NewRecorder()
	h.Download(w, makeFilesReq(http.MethodGet, "/", "", storageID, "folder/a.txt"))
	if w.Code != http.StatusOK || w.Body.String() != "abc" {
		t.Fatalf("download expected 200/abc, got %d/%q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), "attachment; filename=\"a.txt\"") {
		t.Fatalf("unexpected content disposition: %q", w.Header().Get("Content-Disposition"))
	}
}

func TestFilesHandlerDownloadAttachmentWithTrackingCompletesWithoutCancellation(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New().String()
	progressWasProvided := false
	h := NewFilesHandler(&mockFilesService{
		getFileForDownloadFn: func(ctx context.Context, userID, storageID uuid.UUID, path string) (*domain.File, error) {
			return &domain.File{ID: fileID, Path: "folder/a.txt", Size: 3}, nil
		},
		downloadFileToWriterFn: func(ctx context.Context, file *domain.File, w io.Writer, progress *service.DownloadProgress) error {
			progressWasProvided = progress != nil
			_, _ = io.WriteString(w, "abc")
			return nil
		},
	})

	w := httptest.NewRecorder()
	h.Download(w, makeFilesReq(http.MethodGet, "/?download_id=ui-download-1", "", storageID, "folder/a.txt"))

	if w.Code != http.StatusOK || w.Body.String() != "abc" {
		t.Fatalf("download expected 200/abc, got %d/%q", w.Code, w.Body.String())
	}
	if !progressWasProvided {
		t.Fatalf("expected tracked UI download to receive progress object")
	}

	h.mu.RLock()
	tracker, ok := h.downloads["ui-download-1"]
	h.mu.RUnlock()
	if !ok || tracker == nil {
		t.Fatalf("expected tracked download to remain registered for progress polling")
	}
	if !tracker.done || tracker.canceled || tracker.err != nil {
		t.Fatalf("unexpected tracker state: done=%v canceled=%v err=%v", tracker.done, tracker.canceled, tracker.err)
	}
}

func TestFilesHandlerDownloadInlineVideoRange(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New().String()
	rangeCalled := false
	h := NewFilesHandler(&mockFilesService{
		getFileForDownloadFn: func(ctx context.Context, userID, storageID uuid.UUID, path string) (*domain.File, error) {
			return &domain.File{ID: fileID, Path: "movie.mp4", Size: 11}, nil
		},
		exactFileSizeFn: func(ctx context.Context, file *domain.File) (int64, error) {
			return 11, nil
		},
		downloadFileRangeToWriter: func(ctx context.Context, file *domain.File, w io.Writer, start, end, totalSize int64, progress *service.DownloadProgress) error {
			rangeCalled = true
			_, _ = io.WriteString(w, "abc")
			return nil
		},
	})

	req := makeFilesReq(http.MethodGet, "/?inline=1", "", storageID, "movie.mp4")
	req.Header.Set("Range", "bytes=0-2")
	w := httptest.NewRecorder()
	h.Download(w, req)
	if w.Code != http.StatusPartialContent || !rangeCalled || w.Body.String() != "abc" {
		t.Fatalf("inline range download failed: code=%d called=%v body=%q", w.Code, rangeCalled, w.Body.String())
	}
}

func TestFilesHandlerDownloadInlineVideoRangeUsesStoredFileSize(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New().String()
	exactCalled := false
	var gotTotal int64

	h := NewFilesHandler(&mockFilesService{
		getFileForDownloadFn: func(ctx context.Context, userID, storageID uuid.UUID, path string) (*domain.File, error) {
			return &domain.File{ID: fileID, Path: "movie.mp4", Size: 11}, nil
		},
		exactFileSizeFn: func(ctx context.Context, file *domain.File) (int64, error) {
			exactCalled = true
			return 0, errors.New("should not be called")
		},
		downloadFileRangeToWriter: func(ctx context.Context, file *domain.File, w io.Writer, start, end, totalSize int64, progress *service.DownloadProgress) error {
			gotTotal = totalSize
			_, _ = io.WriteString(w, "abc")
			return nil
		},
	})

	req := makeFilesReq(http.MethodGet, "/?inline=1", "", storageID, "movie.mp4")
	req.Header.Set("Range", "bytes=0-2")
	w := httptest.NewRecorder()
	h.Download(w, req)

	if w.Code != http.StatusPartialContent {
		t.Fatalf("inline range download failed: code=%d body=%q", w.Code, w.Body.String())
	}
	if exactCalled {
		t.Fatalf("expected stored file size to avoid exact size lookup")
	}
	if gotTotal != 11 {
		t.Fatalf("unexpected total size forwarded to range download: %d", gotTotal)
	}
}

func TestFilesHandlerDownloadInlineVideoUsesStreamingPathWithoutRange(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New().String()
	streamCalled := false
	downloadCalled := false
	exactCalled := false
	h := NewFilesHandler(&mockFilesService{
		getFileForDownloadFn: func(ctx context.Context, userID, storageID uuid.UUID, path string) (*domain.File, error) {
			return &domain.File{ID: fileID, Path: "movie.mp4", Size: 11}, nil
		},
		exactFileSizeFn: func(ctx context.Context, file *domain.File) (int64, error) {
			exactCalled = true
			return 11, nil
		},
		downloadFileToWriterFn: func(ctx context.Context, file *domain.File, w io.Writer, progress *service.DownloadProgress) error {
			downloadCalled = true
			_, _ = io.WriteString(w, "download")
			return nil
		},
		streamFileToWriterFn: func(ctx context.Context, file *domain.File, w io.Writer, progress *service.DownloadProgress) error {
			streamCalled = true
			_, _ = io.WriteString(w, "stream")
			return nil
		},
	})

	req := makeFilesReq(http.MethodGet, "/?inline=1", "", storageID, "movie.mp4")
	w := httptest.NewRecorder()
	h.Download(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "stream" {
		t.Fatalf("inline full download failed: code=%d body=%q", w.Code, w.Body.String())
	}
	if !streamCalled {
		t.Fatalf("expected streaming path to be used for inline video")
	}
	if downloadCalled {
		t.Fatalf("regular download path should not be used for inline video")
	}
	if exactCalled {
		t.Fatalf("expected inline full stream to avoid exact size lookup")
	}
	if got := w.Header().Get("Content-Length"); got != "11" {
		t.Fatalf("expected stored file size content length, got %q", got)
	}
}

func TestFilesHandlerDownloadInlineVideoWithoutRangeAndUnknownSizeSkipsExactLookup(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New().String()
	streamCalled := false
	exactCalled := false
	h := NewFilesHandler(&mockFilesService{
		getFileForDownloadFn: func(ctx context.Context, userID, storageID uuid.UUID, path string) (*domain.File, error) {
			return &domain.File{ID: fileID, Path: "movie.mp4", Size: 0}, nil
		},
		exactFileSizeFn: func(ctx context.Context, file *domain.File) (int64, error) {
			exactCalled = true
			return 11, nil
		},
		streamFileToWriterFn: func(ctx context.Context, file *domain.File, w io.Writer, progress *service.DownloadProgress) error {
			streamCalled = true
			_, _ = io.WriteString(w, "stream")
			return nil
		},
	})

	req := makeFilesReq(http.MethodGet, "/?inline=1", "", storageID, "movie.mp4")
	w := httptest.NewRecorder()
	h.Download(w, req)

	if w.Code != http.StatusOK || w.Body.String() != "stream" {
		t.Fatalf("inline full download failed: code=%d body=%q", w.Code, w.Body.String())
	}
	if !streamCalled {
		t.Fatalf("expected streaming path to be used for inline video")
	}
	if exactCalled {
		t.Fatalf("expected unknown-size inline stream without range to avoid exact size lookup")
	}
	if got := w.Header().Get("Content-Length"); got != "" {
		t.Fatalf("expected unknown-size inline stream to omit content length, got %q", got)
	}
}

func TestFilesHandlerDownloadInlineVideoOpenRangeExtendsToEOF(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New().String()
	rangeCalled := false
	var gotStart, gotEnd, gotTotal int64
	h := NewFilesHandler(&mockFilesService{
		getFileForDownloadFn: func(ctx context.Context, userID, storageID uuid.UUID, path string) (*domain.File, error) {
			return &domain.File{ID: fileID, Path: "movie.mp4", Size: 11}, nil
		},
		exactFileSizeFn: func(ctx context.Context, file *domain.File) (int64, error) {
			return 11, nil
		},
		downloadFileRangeToWriter: func(ctx context.Context, file *domain.File, w io.Writer, start, end, totalSize int64, progress *service.DownloadProgress) error {
			rangeCalled = true
			gotStart = start
			gotEnd = end
			gotTotal = totalSize
			_, _ = io.WriteString(w, "bcdefghijk")
			return nil
		},
	})

	req := makeFilesReq(http.MethodGet, "/?inline=1", "", storageID, "movie.mp4")
	req.Header.Set("Range", "bytes=1-")
	w := httptest.NewRecorder()
	h.Download(w, req)

	if w.Code != http.StatusPartialContent || !rangeCalled {
		t.Fatalf("inline open range download failed: code=%d called=%v", w.Code, rangeCalled)
	}
	if gotStart != 1 || gotEnd != 10 || gotTotal != 11 {
		t.Fatalf("unexpected open range forwarded to service: %d-%d/%d", gotStart, gotEnd, gotTotal)
	}
	if got := w.Header().Get("Content-Range"); got != "bytes 1-10/11" {
		t.Fatalf("unexpected content-range: %q", got)
	}
	if got := w.Header().Get("Content-Length"); got != "10" {
		t.Fatalf("unexpected content-length: %q", got)
	}
}

func TestFilesHandlerDownloadInlineMKVRange(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New().String()
	rangeCalled := false
	h := NewFilesHandler(&mockFilesService{
		getFileForDownloadFn: func(ctx context.Context, userID, storageID uuid.UUID, path string) (*domain.File, error) {
			return &domain.File{ID: fileID, Path: "movie.mkv", Size: 11}, nil
		},
		exactFileSizeFn: func(ctx context.Context, file *domain.File) (int64, error) {
			return 11, nil
		},
		downloadFileRangeToWriter: func(ctx context.Context, file *domain.File, w io.Writer, start, end, totalSize int64, progress *service.DownloadProgress) error {
			rangeCalled = true
			_, _ = io.WriteString(w, "abc")
			return nil
		},
	})

	req := makeFilesReq(http.MethodGet, "/?inline=1", "", storageID, "movie.mkv")
	req.Header.Set("Range", "bytes=0-2")
	w := httptest.NewRecorder()
	h.Download(w, req)
	if w.Code != http.StatusPartialContent || !rangeCalled || w.Body.String() != "abc" {
		t.Fatalf("inline mkv range download failed: code=%d called=%v body=%q", w.Code, rangeCalled, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "video/x-matroska" {
		t.Fatalf("unexpected mkv content type: %q", got)
	}
}

func TestFilesHandlerDownloadInlineInvalidRange(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New().String()
	h := NewFilesHandler(&mockFilesService{
		getFileForDownloadFn: func(ctx context.Context, userID, storageID uuid.UUID, path string) (*domain.File, error) {
			return &domain.File{ID: fileID, Path: "movie.mp4", Size: 11}, nil
		},
		exactFileSizeFn: func(ctx context.Context, file *domain.File) (int64, error) {
			return 11, nil
		},
	})

	req := makeFilesReq(http.MethodGet, "/?inline=1", "", storageID, "movie.mp4")
	req.Header.Set("Range", "bytes=10-5")
	w := httptest.NewRecorder()
	h.Download(w, req)
	if w.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("expected 416 on invalid range, got %d", w.Code)
	}
}

func TestFilesHandlerDownloadAndUploadValidation(t *testing.T) {
	h := NewFilesHandler(&mockFilesService{})
	storageID := uuid.New().String()

	w := httptest.NewRecorder()
	h.Download(w, makeFilesReq(http.MethodGet, "/", "", storageID, ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("download expected 400 when path missing, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.Upload(w, makeFilesReq(http.MethodPost, "/", "plain body", storageID, ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("upload expected 400 for non-multipart, got %d", w.Code)
	}
}

func TestFilesHandlerUploadSuccess(t *testing.T) {
	uploadDone := make(chan struct{})
	h := NewFilesHandler(&mockFilesService{
		uploadFn: func(ctx context.Context, userID, storageID uuid.UUID, path string, size int64, reader io.Reader, progress *service.UploadProgress, onConflict string) (*domain.File, bool, error) {
			defer close(uploadDone)
			b, _ := io.ReadAll(reader)
			if path != "folder/a.txt" || string(b) != "hello" {
				t.Fatalf("unexpected upload payload: path=%s body=%q", path, string(b))
			}
			if onConflict != service.UploadConflictKeepBoth {
				t.Fatalf("unexpected on_conflict: %q", onConflict)
			}
			return &domain.File{ID: uuid.New(), Path: path}, false, nil
		},
	})

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("path", "folder")
	_ = mw.WriteField("upload_id", "upload-1")
	part, err := mw.CreateFormFile("file", "a.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = io.WriteString(part, "hello")
	_ = mw.Close()

	req := makeFilesReq(http.MethodPost, "/", "", uuid.New().String(), "")
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Body = io.NopCloser(bytes.NewReader(body.Bytes()))
	req.ContentLength = int64(body.Len())

	w := httptest.NewRecorder()
	h.Upload(w, req)
	if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), `"upload_id":"upload-1"`) {
		t.Fatalf("unexpected upload response: code=%d body=%s", w.Code, w.Body.String())
	}

	select {
	case <-uploadDone:
	case <-time.After(2 * time.Second):
		t.Fatalf("upload service was not called")
	}
}

func TestFilesHandlerUploadSkipOnConflict(t *testing.T) {
	h := NewFilesHandler(&mockFilesService{
		uploadFn: func(ctx context.Context, userID, storageID uuid.UUID, path string, size int64, reader io.Reader, progress *service.UploadProgress, onConflict string) (*domain.File, bool, error) {
			if onConflict != service.UploadConflictSkip {
				t.Fatalf("expected skip policy, got %q", onConflict)
			}
			_, _ = io.Copy(io.Discard, reader)
			return nil, true, nil
		},
	})

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("path", "folder")
	_ = mw.WriteField("upload_id", "upload-skip-1")
	_ = mw.WriteField("on_conflict", "skip")
	part, err := mw.CreateFormFile("file", "a.txt")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = io.WriteString(part, "hello")
	_ = mw.Close()

	req := makeFilesReq(http.MethodPost, "/", "", uuid.New().String(), "")
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Body = io.NopCloser(bytes.NewReader(body.Bytes()))
	req.ContentLength = int64(body.Len())

	w := httptest.NewRecorder()
	h.Upload(w, req)
	if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), `"upload_id":"upload-skip-1"`) {
		t.Fatalf("unexpected upload response: code=%d body=%s", w.Code, w.Body.String())
	}

	progressReq := makeFilesReq(http.MethodGet, "/?upload_id=upload-skip-1", "", "", "")
	progressW := httptest.NewRecorder()
	h.UploadProgress(progressW, progressReq)
	if !strings.Contains(progressW.Body.String(), `"status":"skipped"`) {
		t.Fatalf("expected skipped upload SSE, got %q", progressW.Body.String())
	}
}

func TestDownloadErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "legacy 20mb chunk",
			err:  fmt.Errorf("downloading chunk 0: %w (status 400): file is too big", domain.ErrTelegramFileTooBig),
			want: "older 20 MB chunk size",
		},
		{
			name: "secret key mismatch",
			err:  fmt.Errorf("decrypting chunk 2: %w: cipher: message authentication failed", domain.ErrDecryptionFailed),
			want: "current SECRET_KEY",
		},
		{
			name: "worker resolution failure",
			err:  fmt.Errorf("downloading chunk 1: %w (status 400): wrong file identifier", domain.ErrTelegramGetFileFailed),
			want: "currently available workers",
		},
		{
			name: "transient telegram stream failure",
			err:  fmt.Errorf("downloading chunk 409: %w: unexpected EOF", domain.ErrDownloadInterrupted),
			want: "interrupted the download stream",
		},
		{
			name: "fallback generic",
			err:  fmt.Errorf("writing chunk 0: broken pipe"),
			want: "Download failed unexpectedly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := downloadErrorMessage(tt.err)
			if !strings.Contains(got, tt.want) {
				t.Fatalf("expected %q to contain %q", got, tt.want)
			}
		})
	}
}

func TestUploadProgressStatus(t *testing.T) {
	progress := &service.UploadProgress{}

	if got := uploadProgressStatus(progress, false, nil, false); got != "uploading" {
		t.Fatalf("expected uploading, got %q", got)
	}

	// Status stays "uploading" while chunks are still being uploaded,
	// even if verification has started (pipeline verification).
	progress.TotalChunks = 2
	progress.UploadedChunks.Store(1)
	progress.VerificationTotalChunks.Store(1)
	if got := uploadProgressStatus(progress, false, nil, false); got != "uploading" {
		t.Fatalf("expected uploading while chunks remain, got %q", got)
	}

	// Switches to "verifying" only after all chunks are uploaded.
	progress.UploadedChunks.Store(2)
	progress.VerificationTotalChunks.Store(2)
	if got := uploadProgressStatus(progress, false, nil, false); got != "verifying" {
		t.Fatalf("expected verifying, got %q", got)
	}

	if got := uploadProgressStatus(progress, true, nil, false); got != "done" {
		t.Fatalf("expected done, got %q", got)
	}
	if got := uploadProgressStatus(progress, true, errors.New("boom"), false); got != "error" {
		t.Fatalf("expected error, got %q", got)
	}
	if got := uploadProgressStatus(progress, true, nil, true); got != "skipped" {
		t.Fatalf("expected skipped, got %q", got)
	}
}

func TestFilesHandlerCancelDownloadAndProgressValidation(t *testing.T) {
	h := NewFilesHandler(&mockFilesService{})
	storageID := uuid.New()

	w := httptest.NewRecorder()
	req := makeFilesReq(http.MethodDelete, "/", "", "", "")
	rctx := chi.RouteContext(req.Context())
	rctx.URLParams.Add("downloadID", "missing")
	h.CancelDownload(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cancel download expected 404, got %d", w.Code)
	}

	cancelled := false
	h.downloads["d1"] = &downloadTracker{
		storageID: storageID,
		progress:  &service.DownloadProgress{},
		cancel:    func() { cancelled = true },
	}
	w = httptest.NewRecorder()
	req = makeFilesReq(http.MethodDelete, "/", "", "", "")
	rctx = chi.RouteContext(req.Context())
	rctx.URLParams.Add("downloadID", "d1")
	h.CancelDownload(w, req)
	if w.Code != http.StatusNoContent || !cancelled {
		t.Fatalf("cancel download expected 204 and cancel callback, got %d cancelled=%v", w.Code, cancelled)
	}

	w = httptest.NewRecorder()
	h.UploadProgress(w, makeFilesReq(http.MethodGet, "/", "", "", ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("upload progress expected 400, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.DownloadProgress(w, makeFilesReq(http.MethodGet, "/", "", "", ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("download progress expected 400, got %d", w.Code)
	}
}

func TestFilesHandlerCancelUpload(t *testing.T) {
	cancelled := false
	cleaned := false
	mock := &mockFilesService{}
	mock.cleanupCancelledUploadFn = func(ctx context.Context, userID, storageID uuid.UUID, path string) error {
		cleaned = true
		return nil
	}
	h := NewFilesHandler(mock)

	uploadID := "up-1"
	storageID := uuid.New()
	h.uploads[uploadID] = &uploadTracker{
		progress:  &service.UploadProgress{},
		cancel:    func() { cancelled = true },
		storageID: storageID,
		filePath:  "a.txt",
	}

	req := makeFilesReq(http.MethodPost, "/", "", "", "")
	rctx := chi.RouteContext(req.Context())
	rctx.URLParams.Add("uploadID", uploadID)
	w := httptest.NewRecorder()
	h.CancelUpload(w, req)
	if w.Code != http.StatusNoContent || !cancelled {
		t.Fatalf("cancel upload expected 204 and cancel callback, got %d cancelled=%v", w.Code, cancelled)
	}

	// Background cleanup sleeps for 1 second before cleaning up.
	time.Sleep(1100 * time.Millisecond)
	if !cleaned {
		t.Fatalf("expected cleanup call after cancellation")
	}
}

func TestFilesHandlerUploadDownloadDeleteProgressDone(t *testing.T) {
	h := NewFilesHandler(&mockFilesService{})
	storageID := uuid.New()

	h.uploads["u1"] = &uploadTracker{
		progress:  &service.UploadProgress{TotalBytes: 10, TotalChunks: 1},
		storageID: storageID,
		done:      true,
	}
	w := httptest.NewRecorder()
	h.UploadProgress(w, makeFilesReq(http.MethodGet, "/?upload_id=u1", "", "", ""))
	if !strings.Contains(w.Body.String(), `"status":"done"`) {
		t.Fatalf("expected done upload SSE, got %q", w.Body.String())
	}

	h.downloads["d2"] = &downloadTracker{
		progress:  &service.DownloadProgress{TotalBytes: 10, TotalChunks: 1},
		storageID: storageID,
		done:      true,
		canceled:  true,
	}
	w = httptest.NewRecorder()
	h.DownloadProgress(w, makeFilesReq(http.MethodGet, "/?download_id=d2", "", "", ""))
	if !strings.Contains(w.Body.String(), `"status":"cancelled"`) {
		t.Fatalf("expected cancelled download SSE, got %q", w.Body.String())
	}

	h.downloads["d3"] = &downloadTracker{
		progress:  &service.DownloadProgress{TotalBytes: 10, TotalChunks: 1},
		storageID: storageID,
		done:      true,
		err:       fmt.Errorf("downloading chunk 0: %w (status 400): file is too big", domain.ErrTelegramFileTooBig),
	}
	w = httptest.NewRecorder()
	h.DownloadProgress(w, makeFilesReq(http.MethodGet, "/?download_id=d3", "", "", ""))
	if !strings.Contains(w.Body.String(), `"status":"error"`) || !strings.Contains(w.Body.String(), `"error_message":"This file was uploaded with an older 20 MB chunk size`) {
		t.Fatalf("expected friendly download error SSE, got %q", w.Body.String())
	}

	tracker := startDeleteTracker("del-progress", storageID)
	tracker.progress.TotalChunks = 3
	tracker.progress.DeletedChunks.Store(3)
	markDeleteTrackerDone(tracker, nil)
	defer func() {
		deleteRegistry.mu.Lock()
		delete(deleteRegistry.m, "del-progress")
		deleteRegistry.mu.Unlock()
	}()

	w = httptest.NewRecorder()
	h.DeleteProgress(w, makeFilesReq(http.MethodGet, "/?delete_id=del-progress", "", "", ""))
	if !strings.Contains(w.Body.String(), `"status":"done"`) {
		t.Fatalf("expected done delete SSE, got %q", w.Body.String())
	}
}

func TestFilesHandlerUploadProgressVerifying(t *testing.T) {
	h := NewFilesHandler(&mockFilesService{})
	storageID := uuid.New()
	progress := &service.UploadProgress{
		TotalBytes:  10,
		TotalChunks: 2,
	}
	progress.VerificationTotalChunks.Store(2)
	progress.UploadedBytes.Store(10)
	progress.UploadedChunks.Store(2)
	progress.VerifiedChunks.Store(1)

	h.uploads["u-verifying"] = &uploadTracker{
		progress:  progress,
		storageID: storageID,
	}

	req := makeFilesReq(http.MethodGet, "/?upload_id=u-verifying", "", "", "")
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	go func() {
		time.Sleep(700 * time.Millisecond)
		cancel()
	}()

	w := httptest.NewRecorder()
	h.UploadProgress(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `"status":"verifying"`) {
		t.Fatalf("expected verifying upload SSE, got %q", body)
	}
	if !strings.Contains(body, `"verification_total":2`) || !strings.Contains(body, `"verified":1`) {
		t.Fatalf("expected verification counters in upload SSE, got %q", body)
	}
}

func TestFilesHandlerMoveAndCreateFolder(t *testing.T) {
	var movedOld, movedNew, folderPath, folderName string
	h := NewFilesHandler(&mockFilesService{
		moveFn: func(ctx context.Context, userID, storageID uuid.UUID, oldPath, newPath string) error {
			movedOld = oldPath
			movedNew = newPath
			return nil
		},
		createFolderFn: func(ctx context.Context, userID, storageID uuid.UUID, path, name string) error {
			folderPath = path
			folderName = name
			return nil
		},
	})
	storageID := uuid.New().String()

	w := httptest.NewRecorder()
	h.Move(w, makeFilesReq(http.MethodPost, "/", `{"old_path":"a","new_path":"b"}`, storageID, ""))
	if w.Code != http.StatusNoContent || movedOld != "a" || movedNew != "b" {
		t.Fatalf("move expected 204 with args, got %d old=%s new=%s", w.Code, movedOld, movedNew)
	}

	w = httptest.NewRecorder()
	h.CreateFolder(w, makeFilesReq(http.MethodPost, "/", `{"path":"root","folder_name":"docs"}`, storageID, ""))
	if w.Code != http.StatusCreated || folderPath != "root" || folderName != "docs" {
		t.Fatalf("create folder expected 201 with args, got %d path=%s name=%s", w.Code, folderPath, folderName)
	}

	w = httptest.NewRecorder()
	h.Move(w, makeFilesReq(http.MethodPost, "/", `{"old_path":"","new_path":"b"}`, storageID, ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("move expected 400 for empty old_path, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.CreateFolder(w, makeFilesReq(http.MethodPost, "/", `{"path":"root","folder_name":""}`, storageID, ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("create folder expected 400 for empty folder_name, got %d", w.Code)
	}
}

func TestFilesHandlerDownloadDir(t *testing.T) {
	h := NewFilesHandler(&mockFilesService{
		downloadDirFn: func(ctx context.Context, userID, storageID uuid.UUID, dirPath string, w io.Writer, progress *service.DownloadProgress) (string, error) {
			_, _ = io.Copy(w, bytes.NewBufferString("zipdata"))
			return "docs", nil
		},
	})
	storageID := uuid.New().String()
	w := httptest.NewRecorder()
	h.DownloadDir(w, makeFilesReq(http.MethodGet, "/", "", storageID, "root/docs"))
	if w.Code != http.StatusOK || w.Body.String() != "zipdata" {
		t.Fatalf("download dir expected 200/zipdata, got %d/%q", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Disposition"); !strings.Contains(got, `filename="docs.zip"`) {
		t.Fatalf("unexpected download dir content disposition: %q", got)
	}
}

func TestFilesHandlerDownloadDirWithTrackingCompletesWithoutCancellation(t *testing.T) {
	h := NewFilesHandler(&mockFilesService{
		downloadDirFn: func(ctx context.Context, userID, storageID uuid.UUID, dirPath string, w io.Writer, progress *service.DownloadProgress) (string, error) {
			if progress == nil {
				t.Fatalf("expected tracked directory download to receive progress object")
			}
			_, _ = io.Copy(w, bytes.NewBufferString("zipdata"))
			return "docs", nil
		},
	})
	storageID := uuid.New().String()

	w := httptest.NewRecorder()
	h.DownloadDir(w, makeFilesReq(http.MethodGet, "/?download_id=ui-dir-1", "", storageID, "root/docs"))

	if w.Code != http.StatusOK || w.Body.String() != "zipdata" {
		t.Fatalf("download dir expected 200/zipdata, got %d/%q", w.Code, w.Body.String())
	}

	h.mu.RLock()
	tracker, ok := h.downloads["ui-dir-1"]
	h.mu.RUnlock()
	if !ok || tracker == nil {
		t.Fatalf("expected tracked directory download to remain registered for progress polling")
	}
	if !tracker.done || tracker.canceled || tracker.err != nil {
		t.Fatalf("unexpected directory tracker state: done=%v canceled=%v err=%v", tracker.done, tracker.canceled, tracker.err)
	}
}

func TestFilesHandlerDownloadDirUsesFilesZipForRoot(t *testing.T) {
	h := NewFilesHandler(&mockFilesService{
		downloadDirFn: func(ctx context.Context, userID, storageID uuid.UUID, dirPath string, w io.Writer, progress *service.DownloadProgress) (string, error) {
			_, _ = io.Copy(w, bytes.NewBufferString("zipdata"))
			return "files", nil
		},
	})

	w := httptest.NewRecorder()
	h.DownloadDir(w, makeFilesReq(http.MethodGet, "/", "", uuid.New().String(), ""))
	if got := w.Header().Get("Content-Disposition"); !strings.Contains(got, `filename="files.zip"`) {
		t.Fatalf("unexpected root download dir content disposition: %q", got)
	}
}

func TestFilesHandlerDownloadInlineNonVideo(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New().String()
	h := NewFilesHandler(&mockFilesService{
		getFileForDownloadFn: func(ctx context.Context, userID, storageID uuid.UUID, path string) (*domain.File, error) {
			return &domain.File{ID: fileID, Path: "doc.txt", Size: 5}, nil
		},
		downloadFileToWriterFn: func(ctx context.Context, file *domain.File, w io.Writer, progress *service.DownloadProgress) error {
			_, _ = io.WriteString(w, "hello")
			return nil
		},
	})

	w := httptest.NewRecorder()
	h.Download(w, makeFilesReq(http.MethodGet, "/?inline=1", "", storageID, "doc.txt"))
	if w.Code != http.StatusOK || w.Body.String() != "hello" {
		t.Fatalf("inline non-video download expected 200/hello, got %d/%q", w.Code, w.Body.String())
	}
}
