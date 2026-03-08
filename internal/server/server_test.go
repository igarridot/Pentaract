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
