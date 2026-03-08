package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveFileIDByMessageSuccess(t *testing.T) {
	const (
		token  = "TOKEN"
		chatID = int64(3696691277)
		msgID  = int64(99)
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/forwardMessage"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":123,"document":{"file_id":"NEW_FILE_ID"}}}`))
		case strings.Contains(r.URL.Path, "/deleteMessage"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	fileID, err := c.ResolveFileIDByMessage(context.Background(), token, chatID, msgID)
	if err != nil {
		t.Fatalf("resolve file id failed: %v", err)
	}
	if fileID != "NEW_FILE_ID" {
		t.Fatalf("unexpected file id: %q", fileID)
	}
}

func TestResolveFileIDByMessageMissingDocument(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/forwardMessage") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":123,"document":{"file_id":""}}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.ResolveFileIDByMessage(context.Background(), "TOKEN", 1, 1)
	if err == nil || !strings.Contains(err.Error(), "missing document file_id") {
		t.Fatalf("expected missing file_id error, got: %v", err)
	}
}

func TestRateLimitErrorString(t *testing.T) {
	err := &RateLimitError{RetryAfter: 2, Message: "limited"}
	if err.Error() != "limited" {
		t.Fatalf("unexpected error string: %q", err.Error())
	}
}
