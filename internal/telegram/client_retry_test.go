package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
	res, err := c.Upload(context.Background(), "TOKEN", 1, []byte("x"), "a.bin")
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
