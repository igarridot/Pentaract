package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadReturnsAPIErrorWhenNotOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/sendDocument") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":false}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.Upload("TOKEN", 1, []byte("x"), "a.bin")
	if err == nil || !strings.Contains(err.Error(), "telegram sendDocument failed") {
		t.Fatalf("expected sendDocument failed error, got: %v", err)
	}
}

func TestDeleteMessageHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/deleteMessage") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`bad request`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.DeleteMessage("TOKEN", 1, 1)
	if err == nil || !strings.Contains(err.Error(), "telegram deleteMessage error") {
		t.Fatalf("expected delete message error, got: %v", err)
	}
}

func TestResolveFileIDByMessageHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/forwardMessage") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"description":"bad request"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.ResolveFileIDByMessage(context.Background(), "TOKEN", 1, 1)
	if err == nil || !strings.Contains(err.Error(), "forwardMessage failed") {
		t.Fatalf("expected forwardMessage failed error, got: %v", err)
	}
}

func TestDownloadGetFileHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/getFile") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"description":"bad request"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.Download(context.Background(), "TOKEN", "file-id")
	if err == nil || !strings.Contains(err.Error(), "telegram getFile failed") {
		t.Fatalf("expected getFile failed error, got: %v", err)
	}
}

func TestUploadDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/sendDocument") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.Upload("TOKEN", 1, []byte("x"), "a.bin")
	if err == nil || !strings.Contains(err.Error(), "decoding response") {
		t.Fatalf("expected decoding response error, got: %v", err)
	}
}

func TestResolveFileIDByMessageDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/forwardMessage") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.ResolveFileIDByMessage(context.Background(), "TOKEN", 1, 1)
	if err == nil || !strings.Contains(err.Error(), "decoding forwardMessage response") {
		t.Fatalf("expected decode error, got: %v", err)
	}
}

func TestDownloadDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/getFile") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.Download(context.Background(), "TOKEN", "file-id")
	if err == nil || !strings.Contains(err.Error(), "decoding file info") {
		t.Fatalf("expected file info decode error, got: %v", err)
	}
}
