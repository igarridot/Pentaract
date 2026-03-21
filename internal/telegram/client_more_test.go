package telegram

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestBuildUploadEnvelopeProducesMultipartPayload(t *testing.T) {
	prefix, suffix, contentType, err := buildUploadEnvelope(-100123, "chunk.bin")
	if err != nil {
		t.Fatalf("buildUploadEnvelope error: %v", err)
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("parse media type: %v", err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("media type = %q, want multipart/form-data", mediaType)
	}

	payload := append(append([]byte(nil), prefix...), []byte("payload")...)
	payload = append(payload, suffix...)
	reader := multipart.NewReader(strings.NewReader(string(payload)), params["boundary"])

	fields := map[string]string{}
	var document []byte
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("next part: %v", err)
		}

		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read part: %v", err)
		}

		if part.FormName() == "document" {
			document = data
		} else {
			fields[part.FormName()] = string(data)
		}
	}

	if fields["chat_id"] != "-100123" {
		t.Fatalf("chat_id = %q, want -100123", fields["chat_id"])
	}
	if string(document) != "payload" {
		t.Fatalf("document = %q, want payload", string(document))
	}
}

func TestHTTPClientsConfiguredCorrectly(t *testing.T) {
	c := NewClient("http://localhost")

	if c.httpClient.Timeout != 30*time.Second {
		t.Fatalf("upload client timeout = %s, want 30s", c.httpClient.Timeout)
	}
	if c.downloadClient.Timeout != 2*time.Minute {
		t.Fatalf("download client timeout = %s, want 2m", c.downloadClient.Timeout)
	}

	for name, client := range map[string]*http.Client{"upload": c.httpClient, "download": c.downloadClient} {
		transport, ok := client.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("%s: expected *http.Transport, got %T", name, client.Transport)
		}
		if transport.MaxIdleConns != 30 {
			t.Fatalf("%s: MaxIdleConns = %d, want 30", name, transport.MaxIdleConns)
		}
		if transport.MaxIdleConnsPerHost != 20 {
			t.Fatalf("%s: MaxIdleConnsPerHost = %d, want 20", name, transport.MaxIdleConnsPerHost)
		}
		if !transport.ForceAttemptHTTP2 {
			t.Fatalf("%s: expected ForceAttemptHTTP2 to be enabled", name)
		}
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
