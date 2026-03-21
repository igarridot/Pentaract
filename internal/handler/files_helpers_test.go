package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestFileSizeCache(t *testing.T) {
	c := newFileSizeCache()
	id := uuid.New()
	if _, ok := c.get(id); ok {
		t.Fatalf("expected cache miss")
	}
	c.set(id, 123)
	if got, ok := c.get(id); !ok || got != 123 {
		t.Fatalf("unexpected cache hit result: %v %v", got, ok)
	}
}

func TestSetupAndWriteSSE(t *testing.T) {
	w := httptest.NewRecorder()
	flusher, ok := setupSSE(w)
	if !ok || flusher == nil {
		t.Fatalf("expected flusher")
	}
	writeSSE(w, flusher, map[string]any{"status": "ok"})
	body := w.Body.String()
	if !strings.Contains(body, "data: ") || !strings.Contains(body, `"status":"ok"`) {
		t.Fatalf("unexpected sse body: %q", body)
	}
}

func TestSanitizeFilename(t *testing.T) {
	got := sanitizeFilename("a\"b\nc\r")
	if strings.ContainsAny(got, "\"\n\r") {
		t.Fatalf("filename not sanitized: %q", got)
	}
}

func TestParseSingleByteRange(t *testing.T) {
	start, end, err := parseSingleByteRange("bytes=0-9", 100)
	if err != nil || start != 0 || end != 9 {
		t.Fatalf("unexpected fixed range: %d-%d err=%v", start, end, err)
	}
	start, end, err = parseSingleByteRange("bytes=10-", 100)
	if err != nil || start != 10 || end != 99 {
		t.Fatalf("unexpected open range: %d-%d err=%v", start, end, err)
	}
	start, end, err = parseSingleByteRange("bytes=-5", 100)
	if err != nil || start != 95 || end != 99 {
		t.Fatalf("unexpected suffix range: %d-%d err=%v", start, end, err)
	}
	if _, _, err := parseSingleByteRange("items=0-1", 100); err == nil {
		t.Fatalf("expected invalid unit error")
	}
	if _, _, err := parseSingleByteRange("bytes=10-5", 100); err == nil {
		t.Fatalf("expected invalid end error")
	}
}

func TestIsInlineVideo(t *testing.T) {
	if !isInlineVideo("video/mp4", "x.bin") {
		t.Fatalf("expected video by content-type")
	}
	if !isInlineVideo("application/octet-stream", "movie.m4v") {
		t.Fatalf("expected video by extension")
	}
	if !isInlineVideo("application/octet-stream", "movie.mkv") {
		t.Fatalf("expected mkv video by extension")
	}
	if isInlineVideo("application/octet-stream", "doc.txt") {
		t.Fatalf("unexpected video detection")
	}
}

func TestContentTypeForFilename(t *testing.T) {
	if got := contentTypeForFilename("movie.mkv"); got != "video/x-matroska" {
		t.Fatalf("unexpected mkv content type: %q", got)
	}
	if got := contentTypeForFilename("doc.txt"); got == "" {
		t.Fatalf("expected non-empty content type for text file")
	}
}

func TestExtractWildcardPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("*", "/foo%2Fbar")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	got := extractWildcardPath(req)
	if got != "foo/bar" {
		t.Fatalf("unexpected wildcard path: %q", got)
	}
}

func TestSetupDownloadTrackerAndFinish(t *testing.T) {
	h := NewFilesHandler(&mockFilesService{})
	storageID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/?download_id=d1", nil)

	ctx, tracker, cleanup := h.setupDownloadTracker(req, storageID)
	defer cleanup()
	if ctx == nil || tracker == nil {
		t.Fatalf("expected tracker context and tracker")
	}

	h.mu.RLock()
	_, ok := h.downloads["d1"]
	h.mu.RUnlock()
	if !ok {
		t.Fatalf("expected tracker to be registered")
	}

	expectedErr := errors.New("x")
	h.finishTracker(tracker, expectedErr)
	h.mu.RLock()
	done := tracker.done
	gotErr := tracker.err
	h.mu.RUnlock()
	if !done || gotErr != expectedErr {
		t.Fatalf("unexpected tracker final state: done=%v err=%v", done, gotErr)
	}
}

func TestSetupDownloadTrackerReplacesExistingTracker(t *testing.T) {
	h := NewFilesHandler(&mockFilesService{})
	storageID := uuid.New()

	req1 := httptest.NewRequest(http.MethodGet, "/?download_id=d1", nil)
	ctx1, tracker1, cleanup1 := h.setupDownloadTracker(req1, storageID)
	defer cleanup1()

	req2 := httptest.NewRequest(http.MethodGet, "/?download_id=d1", nil)
	_, tracker2, cleanup2 := h.setupDownloadTracker(req2, storageID)
	defer cleanup2()

	if tracker1 == nil || tracker2 == nil || tracker1 == tracker2 {
		t.Fatalf("expected tracker replacement, got first=%p second=%p", tracker1, tracker2)
	}

	select {
	case <-ctx1.Done():
	default:
		t.Fatalf("expected previous tracker context to be canceled")
	}

	h.mu.RLock()
	firstCanceled := tracker1.canceled
	firstDone := tracker1.done
	current := h.downloads["d1"]
	h.mu.RUnlock()

	if !firstCanceled || !firstDone {
		t.Fatalf("expected previous tracker to be marked canceled/done, got canceled=%v done=%v", firstCanceled, firstDone)
	}
	if current != tracker2 {
		t.Fatalf("expected latest tracker to remain registered")
	}

	h.cleanupDownloadTracker("d1", tracker1)
	h.mu.RLock()
	current = h.downloads["d1"]
	h.mu.RUnlock()
	if current != tracker2 {
		t.Fatalf("expected old tracker cleanup to keep latest tracker registered")
	}

	h.cleanupDownloadTracker("d1", tracker2)
	h.mu.RLock()
	_, ok := h.downloads["d1"]
	h.mu.RUnlock()
	if ok {
		t.Fatalf("expected current tracker cleanup to remove download entry")
	}
}
