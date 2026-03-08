package telegram

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadSendsMultipartAndReturnsResult(t *testing.T) {
	const (
		token    = "TOKEN"
		chatID   = int64(3696691277)
		filename = "chunk.bin"
	)
	payload := []byte("hello-telegram")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot"+token+"/sendDocument" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		mr, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("multipart reader: %v", err)
		}

		var gotChatID string
		var gotFileName string
		var gotPayload []byte

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
				b, err := io.ReadAll(part)
				if err != nil {
					t.Fatalf("read chat_id part: %v", err)
				}
				gotChatID = string(b)
			case "document":
				gotFileName = part.FileName()
				b, err := io.ReadAll(part)
				if err != nil {
					t.Fatalf("read document part: %v", err)
				}
				gotPayload = b
			}
		}

		if gotChatID != "-1003696691277" {
			t.Fatalf("unexpected chat_id: %q", gotChatID)
		}
		if gotFileName != filename {
			t.Fatalf("unexpected filename: %q", gotFileName)
		}
		if !bytes.Equal(gotPayload, payload) {
			t.Fatalf("unexpected payload: %q", string(gotPayload))
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":77,"document":{"file_id":"FILE123"}}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	res, err := c.Upload(token, chatID, payload, filename)
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if res.FileID != "FILE123" || res.MessageID != 77 {
		t.Fatalf("unexpected upload result: %+v", res)
	}
}

func TestDeleteMessageSendsConvertedChatIDAndMessageID(t *testing.T) {
	const (
		token     = "TOKEN"
		chatID    = int64(3696691277)
		messageID = int64(55)
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot"+token+"/deleteMessage" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("chat_id"); got != "-1003696691277" {
			t.Fatalf("unexpected chat_id: %q", got)
		}
		if got := r.URL.Query().Get("message_id"); got != "55" {
			t.Fatalf("unexpected message_id: %q", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	if err := c.DeleteMessage(token, chatID, messageID); err != nil {
		t.Fatalf("delete message failed: %v", err)
	}
}

func TestDownloadSuccess(t *testing.T) {
	const (
		token      = "TOKEN"
		fileID     = "FILE_ID"
		downloaded = "payload-bytes"
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			if got := r.URL.Query().Get("file_id"); got != fileID {
				t.Fatalf("unexpected file_id: %q", got)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"path/chunk.bin"}}`))
		case strings.Contains(r.URL.Path, "/file/bot"+token+"/path/chunk.bin"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(downloaded))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	data, err := c.Download(context.Background(), token, fileID)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	if string(data) != downloaded {
		t.Fatalf("unexpected payload: %q", string(data))
	}
}

func TestDownloadFailsWhenGetFileIsNotOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/getFile") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":false}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.Download(context.Background(), "TOKEN", "file-id")
	if err == nil {
		t.Fatalf("expected error when getFile is not OK")
	}
	if !strings.Contains(err.Error(), "telegram getFile failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}
