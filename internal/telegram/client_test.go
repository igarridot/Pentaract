package telegram

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func makeResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestConvertChatID(t *testing.T) {
	if got := convertChatID(3696691277); got != -1003696691277 {
		t.Fatalf("expected converted chat id -1003696691277, got %d", got)
	}
	if got := convertChatID(-1001234567890); got != -1001234567890 {
		t.Fatalf("negative id should remain unchanged, got %d", got)
	}
}

func TestParseRateLimitError(t *testing.T) {
	if got := parseRateLimitError(makeResp(http.StatusOK, `{"ok":true}`)); got != nil {
		t.Fatalf("expected nil for non-429 response")
	}

	withRetryAfter := parseRateLimitError(makeResp(http.StatusTooManyRequests, `{"ok":false,"description":"Too Many Requests","parameters":{"retry_after":7}}`))
	if withRetryAfter == nil {
		t.Fatalf("expected rate limit error")
	}
	if withRetryAfter.RetryAfter != 7 {
		t.Fatalf("expected retry_after=7, got %d", withRetryAfter.RetryAfter)
	}

	fallback := parseRateLimitError(makeResp(http.StatusTooManyRequests, `invalid-json`))
	if fallback == nil {
		t.Fatalf("expected fallback rate limit error")
	}
	if fallback.RetryAfter != 5 {
		t.Fatalf("expected fallback retry_after=5, got %d", fallback.RetryAfter)
	}
}

func TestGenerateChunkFilename(t *testing.T) {
	id := uuid.MustParse("ddeb27fb-d9a0-4624-be4d-4615062daed4")
	got := GenerateChunkFilename(id, 3)
	want := "ddeb27fb-d9a0-4624-be4d-4615062daed4_3"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestDownloadFailsOnFileEndpointHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk.bin"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`telegram down`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	data, err := c.Download(context.Background(), "TOKEN", "file-id")
	if err == nil {
		t.Fatalf("expected error, got nil with data=%q", string(data))
	}
	if !strings.Contains(err.Error(), "telegram file download error") {
		t.Fatalf("unexpected error: %v", err)
	}
}
