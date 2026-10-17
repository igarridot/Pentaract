package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/telegram"
)

// chunkServer serves encrypted chunks keyed by Telegram file id and records
// which ones were downloaded, in order.
type chunkServer struct {
	srv       *httptest.Server
	mu        sync.Mutex
	downloads []string
}

func newChunkServer(encrypted map[string][]byte) *chunkServer {
	s := &chunkServer{}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			id := r.URL.Query().Get("file_id")
			if _, ok := encrypted[id]; !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `{"ok":true,"result":{"file_path":"path/%s.bin"}}`, id)
		case strings.Contains(r.URL.Path, "/file/"):
			id := strings.TrimSuffix(r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:], ".bin")
			s.mu.Lock()
			s.downloads = append(s.downloads, id)
			s.mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(encrypted[id])
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return s
}

func (s *chunkServer) downloaded() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.downloads...)
}

// Files uploaded before the chunk size dropped to UploadChunkSize hold 20 MiB
// per chunk. The inferred range window lands on the wrong byte offset for
// them, which used to serve bytes from the wrong position of the file; the
// downloader must notice and replay the file from its first chunk instead.
func TestStorageManagerDownloadRangeToWriterReplaysLegacyChunkLayout(t *testing.T) {
	fileID := uuid.New()
	storageID := uuid.New()
	legacyChunkSize := UploadChunkSize + UploadChunkSafetyMargin // 20 MiB exactly
	plain0 := bytes.Repeat([]byte("a"), legacyChunkSize)
	plain1 := []byte("tail-data")
	cipher := NewChunkCipher("secret")
	enc0, _, err := cipher.EncryptChunk(fileID, 0, plain0)
	if err != nil {
		t.Fatalf("encrypt chunk 0: %v", err)
	}
	enc1, _, err := cipher.EncryptChunk(fileID, 1, plain1)
	if err != nil {
		t.Fatalf("encrypt chunk 1: %v", err)
	}

	server := newChunkServer(map[string][]byte{"CHUNK0": enc0, "CHUNK1": enc1})
	defer server.srv.Close()

	chunksRepo := newFakeChunksRepo()
	chunksRepo.chunks = []domain.FileChunk{
		{ID: uuid.New(), FileID: fileID, TelegramFileID: "CHUNK0", Position: 0},
		{ID: uuid.New(), FileID: fileID, TelegramFileID: "CHUNK1", Position: 1},
	}
	m := newStorageManager(storageDeps{
		filesRepo:    chunksRepo,
		storagesRepo: fakeStorageGetter{storage: domain.Storage{ID: storageID, ChatID: 123}},
		workersRepo:  &fakeWorkersRepo{},
		scheduler:    NewWorkerScheduler(&fakeManagerSchedulerRepo{}, 10),
		tgClient:     telegram.NewClient(server.srv.URL),
		chunkCipher:  cipher,
	})

	totalSize := int64(len(plain0) + len(plain1))
	start := int64(len(plain0)) + 2
	end := start + 3

	var out bytes.Buffer
	err = m.DownloadRangeToWriter(context.Background(), &domain.File{ID: fileID, StorageID: storageID, Size: totalSize}, &out, start, end, totalSize, nil)
	if err != nil {
		t.Fatalf("download range failed: %v", err)
	}
	if out.String() != "il-d" {
		t.Fatalf("legacy layout served wrong bytes: %q", out.String())
	}
	if got := server.downloaded(); len(got) != 2 {
		t.Fatalf("expected the replay to fetch both chunks, got %v", got)
	}
}

func TestCheckChunkLayout(t *testing.T) {
	const lastPosition int16 = 3
	full := int64(UploadChunkSize)
	tests := []struct {
		name       string
		chunk      domain.FileChunk
		chunkStart int64
		size       int64
		totalSize  int64
		wantErr    bool
	}{
		{name: "full non-final chunk", chunk: domain.FileChunk{Position: 1}, chunkStart: full, size: full, totalSize: 4 * full, wantErr: false},
		{name: "legacy 20 MiB non-final chunk", chunk: domain.FileChunk{Position: 1}, chunkStart: full, size: full + UploadChunkSafetyMargin, totalSize: 4 * full, wantErr: true},
		{name: "short non-final chunk", chunk: domain.FileChunk{Position: 1}, chunkStart: full, size: full - 1, totalSize: 4 * full, wantErr: true},
		{name: "final chunk ends at file size", chunk: domain.FileChunk{Position: lastPosition}, chunkStart: 3 * full, size: 100, totalSize: 3*full + 100, wantErr: false},
		{name: "final chunk under a size that includes multipart framing", chunk: domain.FileChunk{Position: lastPosition}, chunkStart: 3 * full, size: 100, totalSize: 3*full + 100 + 300, wantErr: false},
		{name: "final chunk of a legacy layout", chunk: domain.FileChunk{Position: lastPosition}, chunkStart: 3 * full, size: 100, totalSize: 3*(full+UploadChunkSafetyMargin) + 100, wantErr: true},
		{name: "final chunk past the file size", chunk: domain.FileChunk{Position: lastPosition}, chunkStart: 3 * full, size: 100, totalSize: 3*full + 50, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkChunkLayout(tt.chunk, tt.chunkStart, tt.size, lastPosition, tt.totalSize)
			if tt.wantErr && !errors.Is(err, errChunkLayoutMismatch) {
				t.Fatalf("expected errChunkLayoutMismatch, got %v", err)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
