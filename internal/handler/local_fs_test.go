package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
)

// newTestFilesHandlerWithBase creates a FilesHandler with a custom localBasePath
// for testing (the production constructor auto-detects from /mnt/data).
func newTestFilesHandlerWithBase(svc filesService, basePath string) *FilesHandler {
	h := NewFilesHandler(svc)
	h.localBasePath = basePath
	return h
}

// resolvedTempDir returns a temp directory with symlinks resolved, so that
// safePath prefix checks work correctly on macOS (/var → /private/var).
func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestSafePath(t *testing.T) {
	base := resolvedTempDir(t)

	// Create a subdirectory and file for testing.
	sub := filepath.Join(base, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "file.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("valid relative path", func(t *testing.T) {
		got, err := safePath(base, "sub/file.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != filepath.Join(sub, "file.txt") {
			t.Fatalf("expected %q, got %q", filepath.Join(sub, "file.txt"), got)
		}
	})

	t.Run("empty relative path resolves to base", func(t *testing.T) {
		got, err := safePath(base, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != base {
			t.Fatalf("expected %q, got %q", base, got)
		}
	})

	t.Run("path traversal blocked", func(t *testing.T) {
		_, err := safePath(base, "../../../etc/passwd")
		if err == nil {
			t.Fatal("expected error for path traversal")
		}
	})

	t.Run("empty base path returns error", func(t *testing.T) {
		_, err := safePath("", "anything")
		if err == nil {
			t.Fatal("expected error for empty base path")
		}
	})

	t.Run("nonexistent path returns error", func(t *testing.T) {
		_, err := safePath(base, "does_not_exist")
		if err == nil {
			t.Fatal("expected error for nonexistent path")
		}
	})
}

func TestBrowseLocalFS(t *testing.T) {
	base := resolvedTempDir(t)
	if err := os.WriteFile(filepath.Join(base, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(base, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("lists files and directories", func(t *testing.T) {
		h := newTestFilesHandlerWithBase(&mockFilesService{}, base)
		req := makeFilesReq(http.MethodGet, "/api/local_fs/browse?path=", "", "", "")
		w := httptest.NewRecorder()
		h.BrowseLocalFS(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
		}

		var elements []domain.FSElement
		if err := json.Unmarshal(w.Body.Bytes(), &elements); err != nil {
			t.Fatalf("bad json: %v", err)
		}
		if len(elements) != 2 {
			t.Fatalf("expected 2 elements, got %d", len(elements))
		}

		byName := map[string]domain.FSElement{}
		for _, e := range elements {
			byName[e.Name] = e
		}

		file, ok := byName["a.txt"]
		if !ok || !file.IsFile || file.Size != 5 {
			t.Fatalf("unexpected file entry: %+v", file)
		}
		dir, ok := byName["subdir"]
		if !ok || dir.IsFile {
			t.Fatalf("unexpected dir entry: %+v", dir)
		}
	})

	t.Run("forbidden when localBasePath is empty", func(t *testing.T) {
		h := newTestFilesHandlerWithBase(&mockFilesService{}, "")
		w := httptest.NewRecorder()
		h.BrowseLocalFS(w, makeFilesReq(http.MethodGet, "/api/local_fs/browse", "", "", ""))

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", w.Code)
		}
	})

	t.Run("path traversal blocked", func(t *testing.T) {
		h := newTestFilesHandlerWithBase(&mockFilesService{}, base)
		w := httptest.NewRecorder()
		h.BrowseLocalFS(w, makeFilesReq(http.MethodGet, "/api/local_fs/browse?path=../../etc", "", "", ""))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
		}
	})
}

func TestUploadLocal(t *testing.T) {
	base := resolvedTempDir(t)
	if err := os.WriteFile(filepath.Join(base, "test.dat"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(base, "mydir"), 0o755); err != nil {
		t.Fatal(err)
	}

	storageID := uuid.New().String()

	t.Run("returns 202 with upload_id", func(t *testing.T) {
		h := newTestFilesHandlerWithBase(&mockFilesService{}, base)
		body := `{"local_path":"test.dat","dest_path":"docs","upload_id":"up-local-1"}`
		req := makeFilesReq(http.MethodPost, "/", body, storageID, "")
		w := httptest.NewRecorder()
		h.UploadLocal(w, req)

		if w.Code != http.StatusAccepted {
			t.Fatalf("expected 202, got %d body=%s", w.Code, w.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("bad json: %v", err)
		}
		if resp["upload_id"] != "up-local-1" {
			t.Fatalf("unexpected upload_id: %v", resp["upload_id"])
		}
	})

	t.Run("forbidden when localBasePath is empty", func(t *testing.T) {
		h := newTestFilesHandlerWithBase(&mockFilesService{}, "")
		body := `{"local_path":"test.dat","dest_path":"docs"}`
		req := makeFilesReq(http.MethodPost, "/", body, storageID, "")
		w := httptest.NewRecorder()
		h.UploadLocal(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", w.Code)
		}
	})

	t.Run("rejects empty local_path", func(t *testing.T) {
		h := newTestFilesHandlerWithBase(&mockFilesService{}, base)
		body := `{"local_path":"","dest_path":"docs"}`
		req := makeFilesReq(http.MethodPost, "/", body, storageID, "")
		w := httptest.NewRecorder()
		h.UploadLocal(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("rejects directory as local_path", func(t *testing.T) {
		h := newTestFilesHandlerWithBase(&mockFilesService{}, base)
		body := `{"local_path":"mydir","dest_path":"docs"}`
		req := makeFilesReq(http.MethodPost, "/", body, storageID, "")
		w := httptest.NewRecorder()
		h.UploadLocal(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("rejects path traversal", func(t *testing.T) {
		h := newTestFilesHandlerWithBase(&mockFilesService{}, base)
		body := `{"local_path":"../../../etc/passwd","dest_path":"docs"}`
		req := makeFilesReq(http.MethodPost, "/", body, storageID, "")
		w := httptest.NewRecorder()
		h.UploadLocal(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}

func TestUploadLocalBatch(t *testing.T) {
	base := resolvedTempDir(t)
	if err := os.WriteFile(filepath.Join(base, "a.txt"), []byte("aaa"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "b.txt"), []byte("bbb"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(base, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}

	storageID := uuid.New().String()

	t.Run("returns upload_ids for each item", func(t *testing.T) {
		h := newTestFilesHandlerWithBase(&mockFilesService{}, base)
		body := `{"items":[{"local_path":"a.txt","dest_path":""},{"local_path":"b.txt","dest_path":"docs"}]}`
		req := makeFilesReq(http.MethodPost, "/", body, storageID, "")
		w := httptest.NewRecorder()
		h.UploadLocalBatch(w, req)

		if w.Code != http.StatusAccepted {
			t.Fatalf("expected 202, got %d body=%s", w.Code, w.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("bad json: %v", err)
		}
		uploads, ok := resp["uploads"].([]any)
		if !ok || len(uploads) != 2 {
			t.Fatalf("expected 2 uploads, got %v", resp["uploads"])
		}
	})

	t.Run("forbidden when localBasePath is empty", func(t *testing.T) {
		h := newTestFilesHandlerWithBase(&mockFilesService{}, "")
		body := `{"items":[{"local_path":"a.txt","dest_path":""}]}`
		req := makeFilesReq(http.MethodPost, "/", body, storageID, "")
		w := httptest.NewRecorder()
		h.UploadLocalBatch(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", w.Code)
		}
	})

	t.Run("rejects empty items", func(t *testing.T) {
		h := newTestFilesHandlerWithBase(&mockFilesService{}, base)
		body := `{"items":[]}`
		req := makeFilesReq(http.MethodPost, "/", body, storageID, "")
		w := httptest.NewRecorder()
		h.UploadLocalBatch(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("rejects batch exceeding 100 items", func(t *testing.T) {
		h := newTestFilesHandlerWithBase(&mockFilesService{}, base)
		items := make([]map[string]string, 101)
		for i := range items {
			items[i] = map[string]string{"local_path": "a.txt", "dest_path": ""}
		}
		itemsJSON, _ := json.Marshal(items)
		body := `{"items":` + string(itemsJSON) + `}`
		req := makeFilesReq(http.MethodPost, "/", body, storageID, "")
		w := httptest.NewRecorder()
		h.UploadLocalBatch(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("rejects directory in batch item", func(t *testing.T) {
		h := newTestFilesHandlerWithBase(&mockFilesService{}, base)
		body := `{"items":[{"local_path":"dir","dest_path":""}]}`
		req := makeFilesReq(http.MethodPost, "/", body, storageID, "")
		w := httptest.NewRecorder()
		h.UploadLocalBatch(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("rejects path traversal in batch item", func(t *testing.T) {
		h := newTestFilesHandlerWithBase(&mockFilesService{}, base)
		body := `{"items":[{"local_path":"../../../etc/passwd","dest_path":""}]}`
		req := makeFilesReq(http.MethodPost, "/", body, storageID, "")
		w := httptest.NewRecorder()
		h.UploadLocalBatch(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("rejects empty local_path in batch item", func(t *testing.T) {
		h := newTestFilesHandlerWithBase(&mockFilesService{}, base)
		body := `{"items":[{"local_path":"","dest_path":""}]}`
		req := makeFilesReq(http.MethodPost, "/", body, storageID, "")
		w := httptest.NewRecorder()
		h.UploadLocalBatch(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})
}
