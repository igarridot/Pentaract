package server

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccessLogMiddlewareIncludesUserAgent(t *testing.T) {
	var logOutput bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{})))
	defer slog.SetDefault(previous)

	handler := accessLogMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, "denied")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/storages/storage-1/files/download/movie.mkv?inline=1", nil)
	req.RemoteAddr = "192.0.2.15:4321"
	req.Header.Set("User-Agent", "Pentaract-Kodi/1.2.3")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	logLine := logOutput.String()
	if !strings.Contains(logLine, "msg=\"http request\"") {
		t.Fatalf("expected access log message, got %q", logLine)
	}
	if !strings.Contains(logLine, "status=401") {
		t.Fatalf("expected status in access log, got %q", logLine)
	}
	if !strings.Contains(logLine, "user_agent=Pentaract-Kodi/1.2.3") {
		t.Fatalf("expected user agent in access log, got %q", logLine)
	}
	if !strings.Contains(logLine, "request=\"GET /api/storages/storage-1/files/download/movie.mkv?inline=1 HTTP/1.1\"") {
		t.Fatalf("expected request line in access log, got %q", logLine)
	}
}
