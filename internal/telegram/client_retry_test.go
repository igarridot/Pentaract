package telegram

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"
)

func withNoSleep(t *testing.T) {
	t.Helper()
	orig := telegramSleep
	telegramSleep = func(time.Duration) {}
	t.Cleanup(func() { telegramSleep = orig })
}

func TestUploadRetriesOn429(t *testing.T) {
	withNoSleep(t)
	attempt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/sendDocument") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		attempt++
		if attempt == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"parameters":{"retry_after":1}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":10,"document":{"file_id":"F"}}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	res, err := c.Upload("TOKEN", 1, []byte("x"), "a.bin")
	if err != nil || res == nil || attempt < 2 {
		t.Fatalf("expected retry success, res=%v err=%v attempts=%d", res, err, attempt)
	}
}

func TestDeleteMessageRetriesOn429(t *testing.T) {
	withNoSleep(t)
	attempt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/deleteMessage") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		attempt++
		if attempt == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"parameters":{"retry_after":1}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	if err := c.DeleteMessage("TOKEN", 1, 1); err != nil || attempt < 2 {
		t.Fatalf("expected retry success, err=%v attempts=%d", err, attempt)
	}
}

func TestResolveFileIDByMessageRetriesOn429(t *testing.T) {
	withNoSleep(t)
	attempt := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/forwardMessage"):
			attempt++
			if attempt == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"ok":false,"parameters":{"retry_after":1}}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"document":{"file_id":"FID"}}}`))
		case strings.Contains(r.URL.Path, "/deleteMessage"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	fileID, err := c.ResolveFileIDByMessage(context.Background(), "TOKEN", 1, 1)
	if err != nil || fileID != "FID" || attempt < 2 {
		t.Fatalf("expected retry success, fileID=%q err=%v attempts=%d", fileID, err, attempt)
	}
}

func TestDownloadRetriesOn429(t *testing.T) {
	withNoSleep(t)
	getFileAttempts := 0
	downloadAttempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			getFileAttempts++
			if getFileAttempts == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"ok":false,"parameters":{"retry_after":1}}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk.bin"):
			downloadAttempts++
			if downloadAttempts == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"ok":false,"parameters":{"retry_after":1}}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`payload`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	data, err := c.Download(context.Background(), "TOKEN", "FILE")
	if err != nil || string(data) != "payload" || getFileAttempts < 2 || downloadAttempts < 2 {
		t.Fatalf("expected retry success, err=%v data=%q getFileAttempts=%d downloadAttempts=%d", err, string(data), getFileAttempts, downloadAttempts)
	}
}

func TestDownloadRetriesOnUnexpectedEOF(t *testing.T) {
	getFileAttempts := 0
	downloadAttempts := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			getFileAttempts++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk.bin"):
			downloadAttempts++
			if downloadAttempts == 1 {
				h, ok := w.(http.Hijacker)
				if !ok {
					t.Fatalf("response writer does not support hijacking")
				}
				conn, bufrw, err := h.Hijack()
				if err != nil {
					t.Fatalf("hijack response: %v", err)
				}
				_, _ = bufrw.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\nshort")
				_ = bufrw.Flush()
				_ = conn.Close()
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`payload-ok`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	data, err := c.Download(context.Background(), "TOKEN", "FILE")
	if err != nil || string(data) != "payload-ok" || getFileAttempts < 1 || downloadAttempts < 2 {
		t.Fatalf("expected retry success after unexpected EOF, err=%v data=%q getFileAttempts=%d downloadAttempts=%d", err, string(data), getFileAttempts, downloadAttempts)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestDownloadRetriesOnTransientGetFileError(t *testing.T) {
	getFileAttempts := 0
	downloadAttempts := 0

	c := NewClient("https://telegram.invalid")
	c.httpClient = &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(r.URL.Path, "/getFile"):
				getFileAttempts++
				if getFileAttempts == 1 {
					return nil, &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":{"file_path":"path/chunk.bin"}}`)),
					Header:     make(http.Header),
				}, nil
			case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk.bin"):
				downloadAttempts++
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`payload-ok`)),
					Header:     make(http.Header),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader(`not found`)),
					Header:     make(http.Header),
				}, nil
			}
		}),
	}

	data, err := c.Download(context.Background(), "TOKEN", "FILE")
	if err != nil || string(data) != "payload-ok" || getFileAttempts < 2 || downloadAttempts != 1 {
		t.Fatalf("expected getFile retry success, err=%v data=%q getFileAttempts=%d downloadAttempts=%d", err, string(data), getFileAttempts, downloadAttempts)
	}
}

func TestDownloadRetriesOnGetFileUnexpectedEOF(t *testing.T) {
	getFileAttempts := 0
	downloadAttempts := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			getFileAttempts++
			if getFileAttempts == 1 {
				h, ok := w.(http.Hijacker)
				if !ok {
					t.Fatalf("response writer does not support hijacking")
				}
				conn, bufrw, err := h.Hijack()
				if err != nil {
					t.Fatalf("hijack response: %v", err)
				}
				_, _ = bufrw.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 64\r\n\r\n{\"ok\":true")
				_ = bufrw.Flush()
				_ = conn.Close()
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/botTOKEN/path/chunk.bin"):
			downloadAttempts++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`payload-ok`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	data, err := c.Download(context.Background(), "TOKEN", "FILE")
	if err != nil || string(data) != "payload-ok" || getFileAttempts < 2 || downloadAttempts != 1 {
		t.Fatalf("expected retry success after getFile unexpected EOF, err=%v data=%q getFileAttempts=%d downloadAttempts=%d", err, string(data), getFileAttempts, downloadAttempts)
	}
}

func TestTransientRetryDelay(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: 250 * time.Millisecond},
		{attempt: 1, want: 500 * time.Millisecond},
		{attempt: 2, want: 750 * time.Millisecond},
	}

	for _, tc := range tests {
		if got := transientRetryDelay(tc.attempt); got != tc.want {
			t.Fatalf("transientRetryDelay(%d) = %s, want %s", tc.attempt, got, tc.want)
		}
	}
}
