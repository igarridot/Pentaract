package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/Dominux/Pentaract/internal/config"
)

func withTempWorkdir(t *testing.T, fn func()) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	fn()
}

func TestServeUIServesExistingAsset(t *testing.T) {
	withTempWorkdir(t, func() {
		uiDir := filepath.Join("ui", "dist")
		if err := os.MkdirAll(uiDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(uiDir, "index.html"), []byte("INDEX"), 0o644); err != nil {
			t.Fatalf("write index: %v", err)
		}
		if err := os.WriteFile(filepath.Join(uiDir, "app.js"), []byte("console.log('ok')"), 0o644); err != nil {
			t.Fatalf("write app: %v", err)
		}

		r := chi.NewRouter()
		serveUI(r)

		req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		if body := rr.Body.String(); body != "console.log('ok')" {
			t.Fatalf("unexpected body: %q", body)
		}
	})
}

func TestServeUIFallsBackToIndexOnMissingAsset(t *testing.T) {
	withTempWorkdir(t, func() {
		uiDir := filepath.Join("ui", "dist")
		if err := os.MkdirAll(uiDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(uiDir, "index.html"), []byte("INDEX"), 0o644); err != nil {
			t.Fatalf("write index: %v", err)
		}

		r := chi.NewRouter()
		serveUI(r)

		req := httptest.NewRequest(http.MethodGet, "/missing-asset.js", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 fallback, got %d", rr.Code)
		}
		if body := rr.Body.String(); body != "INDEX" {
			t.Fatalf("expected index fallback, got %q", body)
		}
	})
}

func TestNewBuildsRouterAndHandlesRequest(t *testing.T) {
	cfg := &config.Config{
		SecretKey:              "secret",
		AccessTokenExpireInSec: 3600,
		SuperuserEmail:         "admin@example.com",
		TelegramAPIBaseURL:     "http://localhost",
		TelegramRateLimit:      10,
	}
	h := New(cfg, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request from empty login body, got %d", rr.Code)
	}
}

func TestNewProtectedRoutesRequireValidBearerToken(t *testing.T) {
	cfg := &config.Config{
		SecretKey:              "secret",
		AccessTokenExpireInSec: 3600,
		SuperuserEmail:         "admin@example.com",
		TelegramAPIBaseURL:     "http://localhost",
		TelegramRateLimit:      10,
	}
	h := New(cfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/storages", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized without token, got %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/storages", nil)
	req.Header.Set("Authorization", "Bearer invalid.token")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized with invalid token, got %d", rr.Code)
	}
}

// The watch progress routes share the /files/ prefix with the catch-all
// DELETE /files/*. A user's own file or folder named "watched" or
// "watch_progress" must still be deletable, and the existing transfer
// routes must resolve exactly as before.
func TestWatchProgressRoutesLeaveExistingFileRoutesIntact(t *testing.T) {
	cfg := &config.Config{
		SecretKey:              "secret",
		AccessTokenExpireInSec: 3600,
		SuperuserEmail:         "admin@example.com",
		TelegramAPIBaseURL:     "http://localhost",
		TelegramRateLimit:      10,
	}
	mux, ok := New(cfg, nil).(*chi.Mux)
	if !ok {
		t.Fatalf("expected the router to be a *chi.Mux")
	}

	const files = "/api/storages/s1/files"
	cases := []struct{ method, path, want string }{
		{http.MethodDelete, files + "/watched", "/api/storages/{storageID}/files/*"},
		{http.MethodDelete, files + "/watched/movie.mkv", "/api/storages/{storageID}/files/*"},
		{http.MethodDelete, files + "/watch_progress/movie.mkv", "/api/storages/{storageID}/files/*"},
		{http.MethodDelete, files + "/movies/movie.mkv", "/api/storages/{storageID}/files/*"},
		{http.MethodGet, files + "/download/watched/movie.mkv", "/api/storages/{storageID}/files/download/*"},
		{http.MethodGet, files + "/tree/watched/", "/api/storages/{storageID}/files/tree/*"},
		{http.MethodGet, files + "/hls/watched/movie.mkv/index.m3u8", "/api/storages/{storageID}/files/hls/*"},
		{http.MethodPost, files + "/upload", "/api/storages/{storageID}/files/upload"},
		{http.MethodPut, files + "/watched/movie.mkv", "/api/storages/{storageID}/files/watched/*"},
		{http.MethodPut, files + "/watch_progress/movie.mkv", "/api/storages/{storageID}/files/watch_progress/*"},
		{http.MethodDelete, files + "/clear_watch_progress/tv/", "/api/storages/{storageID}/files/*"},
		{http.MethodPost, files + "/clear_watch_progress/tv/", "/api/storages/{storageID}/files/clear_watch_progress/*"},
		{http.MethodDelete, "/api/watch_progress", "/api/watch_progress"},
	}
	for _, tc := range cases {
		got := mux.Find(chi.NewRouteContext(), tc.method, tc.path)
		if got != tc.want {
			t.Errorf("%s %s routed to %q, want %q", tc.method, tc.path, got, tc.want)
		}
	}
}
