package telegram

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Dominux/Pentaract/internal/domain"
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

func TestUploadSuccessWithHTTPRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/sendDocument") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		mr, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("multipart reader: %v", err)
		}
		var gotChatID string
		var docBytes []byte
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("next part: %v", err)
			}
			switch part.FormName() {
			case "chat_id":
				b, _ := io.ReadAll(part)
				gotChatID = string(b)
			case "document":
				docBytes, _ = io.ReadAll(part)
			}
			part.Close()
		}
		if gotChatID != "-100999" {
			t.Fatalf("expected chat_id=-100999, got %q", gotChatID)
		}
		if string(docBytes) != "payload" {
			t.Fatalf("expected document=payload, got %q", string(docBytes))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":42,"document":{"file_id":"FILE123"}}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	result, err := c.Upload(context.Background(), "TOKEN", 999, []byte("payload"), "test.bin")
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if result.FileID != "FILE123" {
		t.Fatalf("expected file_id=FILE123, got %q", result.FileID)
	}
	if result.MessageID != 42 {
		t.Fatalf("expected message_id=42, got %d", result.MessageID)
	}
}

func TestUploadRetryOn429(t *testing.T) {
	origSleep := telegramSleep
	telegramSleep = func(d time.Duration) {} // no-op sleep for test speed
	defer func() { telegramSleep = origSleep }()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/sendDocument") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Drain the body to avoid broken pipe errors
		_, _ = io.ReadAll(r.Body)
		r.Body.Close()

		n := attempts.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"description":"Too Many Requests","parameters":{"retry_after":1}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":10,"document":{"file_id":"AFTER_RETRY"}}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	result, err := c.Upload(context.Background(), "TOKEN", 999, []byte("data"), "chunk.bin")
	if err != nil {
		t.Fatalf("upload after retries failed: %v", err)
	}
	if result.FileID != "AFTER_RETRY" {
		t.Fatalf("expected AFTER_RETRY, got %q", result.FileID)
	}
	if attempts.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts.Load())
	}
}

func TestDownloadSuccessWithHTTPRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"docs/file.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/docs/file.bin"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("file-content-here"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	data, err := c.Download(context.Background(), "TOKEN", "my-file-id")
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	if string(data) != "file-content-here" {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestDownloadRetryOn429(t *testing.T) {
	origSleep := telegramSleep
	telegramSleep = func(d time.Duration) {}
	defer func() { telegramSleep = origSleep }()

	var getFileAttempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			n := getFileAttempts.Add(1)
			if n == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"ok":false,"description":"Too Many Requests","parameters":{"retry_after":2}}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/f.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/f.bin"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok-data"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	data, err := c.Download(context.Background(), "TOKEN", "fid")
	if err != nil {
		t.Fatalf("download with retry failed: %v", err)
	}
	if string(data) != "ok-data" {
		t.Fatalf("unexpected data: %q", string(data))
	}
	if getFileAttempts.Load() != 2 {
		t.Fatalf("expected 2 getFile attempts, got %d", getFileAttempts.Load())
	}
}

func TestDownloadGetFileFailureReturnsSentinelError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/getFile") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: wrong file identifier"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.Download(context.Background(), "TOKEN", "bad-file-id")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, domain.ErrTelegramGetFileFailed) {
		t.Fatalf("expected ErrTelegramGetFileFailed, got: %v", err)
	}
}

func TestDownloadGetFileTooBigReturnsSentinelError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/getFile") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: file is too big"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.Download(context.Background(), "TOKEN", "large-file-id")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, domain.ErrTelegramFileTooBig) {
		t.Fatalf("expected ErrTelegramFileTooBig, got: %v", err)
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

func TestDeleteMessageSuccessWithHTTPRoundTrip(t *testing.T) {
	var requestPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path + "?" + r.URL.RawQuery
		if strings.Contains(r.URL.Path, "/deleteMessage") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.DeleteMessage("TOKEN", 999, 42)
	if err != nil {
		t.Fatalf("deleteMessage failed: %v", err)
	}
	if !strings.Contains(requestPath, "chat_id=-100999") {
		t.Fatalf("expected chat_id=-100999 in request, got %q", requestPath)
	}
	if !strings.Contains(requestPath, "message_id=42") {
		t.Fatalf("expected message_id=42 in request, got %q", requestPath)
	}
}

func TestDeleteMessageRetryOn429(t *testing.T) {
	origSleep := telegramSleep
	telegramSleep = func(d time.Duration) {}
	defer func() { telegramSleep = origSleep }()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/deleteMessage") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		n := attempts.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"description":"Too Many Requests","parameters":{"retry_after":1}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.DeleteMessage("TOKEN", 999, 42)
	if err != nil {
		t.Fatalf("deleteMessage retry failed: %v", err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts.Load())
	}
}

func TestResolveFileIDByMessageSuccess(t *testing.T) {
	var deleteRequest string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/forwardMessage"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":88,"document":{"file_id":"RESOLVED_FILE_ID"}}}`))
		case strings.Contains(r.URL.Path, "/deleteMessage"):
			deleteRequest = r.URL.RawQuery
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	fileID, err := c.ResolveFileIDByMessage(context.Background(), "TOKEN", 999, 77)
	if err != nil {
		t.Fatalf("resolveFileIDByMessage failed: %v", err)
	}
	if fileID != "RESOLVED_FILE_ID" {
		t.Fatalf("expected RESOLVED_FILE_ID, got %q", fileID)
	}
	// Verify the forwarded message copy was cleaned up
	if !strings.Contains(deleteRequest, "message_id=88") {
		t.Fatalf("expected delete of forwarded copy (message_id=88), got %q", deleteRequest)
	}
}

func TestResolveFileIDByMessageFailureReturnsSentinelError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/forwardMessage") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: message not found"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.ResolveFileIDByMessage(context.Background(), "TOKEN", 999, 77)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, domain.ErrTelegramResolveFailed) {
		t.Fatalf("expected ErrTelegramResolveFailed, got: %v", err)
	}
}

func TestResolveFileIDByMessageRetryOn429(t *testing.T) {
	origSleep := telegramSleep
	telegramSleep = func(d time.Duration) {}
	defer func() { telegramSleep = origSleep }()

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/forwardMessage"):
			n := attempts.Add(1)
			if n == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"ok":false,"description":"Too Many Requests","parameters":{"retry_after":1}}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":88,"document":{"file_id":"FID"}}}`))
		case strings.Contains(r.URL.Path, "/deleteMessage"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	fileID, err := c.ResolveFileIDByMessage(context.Background(), "TOKEN", 999, 77)
	if err != nil {
		t.Fatalf("resolveFileIDByMessage with retry failed: %v", err)
	}
	if fileID != "FID" {
		t.Fatalf("expected FID, got %q", fileID)
	}
	if attempts.Load() != 2 {
		t.Fatalf("expected 2 forwardMessage attempts, got %d", attempts.Load())
	}
}

func TestParseRateLimitErrorWith429HTTPTestServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"ok":false,"description":"Too Many Requests: retry after 12","parameters":{"retry_after":12}}`))
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	rlErr := parseRateLimitError(resp)
	if rlErr == nil {
		t.Fatalf("expected rate limit error from 429 server response")
	}
	if rlErr.RetryAfter != 12 {
		t.Fatalf("expected retry_after=12, got %d", rlErr.RetryAfter)
	}
}

func TestDownloadFileDownloadRetryOn429(t *testing.T) {
	origSleep := telegramSleep
	telegramSleep = func(d time.Duration) {}
	defer func() { telegramSleep = origSleep }()

	var downloadAttempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/f.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/f.bin"):
			n := downloadAttempts.Add(1)
			if n == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"ok":false,"description":"Too Many Requests","parameters":{"retry_after":1}}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("finally"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	data, err := c.Download(context.Background(), "TOKEN", "fid")
	if err != nil {
		t.Fatalf("download file retry failed: %v", err)
	}
	if string(data) != "finally" {
		t.Fatalf("unexpected data: %q", string(data))
	}
	if downloadAttempts.Load() != 2 {
		t.Fatalf("expected 2 download attempts, got %d", downloadAttempts.Load())
	}
}
