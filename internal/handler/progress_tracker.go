package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/service"
)

type fileSizeCache struct {
	mu    sync.RWMutex
	sizes map[uuid.UUID]int64
}

func newFileSizeCache() *fileSizeCache {
	return &fileSizeCache{sizes: make(map[uuid.UUID]int64)}
}

func (c *fileSizeCache) get(fileID uuid.UUID) (int64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	size, ok := c.sizes[fileID]
	return size, ok
}

func (c *fileSizeCache) set(fileID uuid.UUID, size int64) {
	c.mu.Lock()
	c.sizes[fileID] = size
	c.mu.Unlock()
}

type uploadTracker struct {
	progress  *service.UploadProgress
	cancel    context.CancelFunc
	storageID uuid.UUID
	filePath  string
	err       error
	skipped   bool
	done      bool
}

type downloadTracker struct {
	storageID uuid.UUID
	progress  *service.DownloadProgress
	cancel    context.CancelFunc
	canceled  bool
	err       error
	done      bool
}

type flushWriter struct {
	w       io.Writer
	flusher http.Flusher
}

func (w *flushWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if n > 0 && w.flusher != nil {
		w.flusher.Flush()
	}
	return n, err
}

var inlineVideoContentTypesByExtension = map[string]string{
	".avi":  "video/x-msvideo",
	".flv":  "video/x-flv",
	".m2ts": "video/mp2t",
	".m4v":  "video/x-m4v",
	".mkv":  "video/x-matroska",
	".mov":  "video/quicktime",
	".mp4":  "video/mp4",
	".mpeg": "video/mpeg",
	".mpg":  "video/mpeg",
	".mts":  "video/mp2t",
	".ogg":  "video/ogg",
	".ts":   "video/mp2t",
	".webm": "video/webm",
	".wmv":  "video/x-ms-wmv",
}

func downloadErrorMessage(err error) string {
	if err == nil {
		return ""
	}

	switch {
	case errors.Is(err, domain.ErrTelegramFileTooBig):
		return "This file was uploaded with an older 20 MB chunk size and Telegram now refuses to download one of its chunks. Re-upload the file to repair it."
	case errors.Is(err, domain.ErrDecryptionFailed):
		return "This file could not be decrypted with the current SECRET_KEY. If the key changed after upload, restore the original key or re-upload the file."
	case errors.Is(err, domain.ErrTelegramGetFileFailed),
		errors.Is(err, domain.ErrTelegramResolveFailed):
		return "Telegram could not resolve at least one chunk with the currently available workers. Check that the original bot still exists and still has access to the channel, or re-upload the file."
	case errors.Is(err, domain.ErrDownloadInterrupted):
		return "Telegram interrupted the download stream for one of the chunks. Please try again."
	default:
		return "Download failed unexpectedly. Please try again."
	}
}

func uploadProgressStatus(progress *service.UploadProgress, done bool, err error, skipped bool) string {
	switch {
	case done && err != nil:
		return "error"
	case done && skipped:
		return "skipped"
	case done:
		return "done"
	case progress != nil && progress.VerificationTotalChunks > 0:
		return "verifying"
	default:
		return "uploading"
	}
}

// setupSSE configures response headers for Server-Sent Events and returns the flusher.
func setupSSE(w http.ResponseWriter) (http.Flusher, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	return flusher, true
}

// writeSSE marshals data as JSON and writes it as an SSE event.
func writeSSE(w http.ResponseWriter, flusher http.Flusher, data any) {
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", b)
	flusher.Flush()
}

// pollSSE is a generic SSE polling loop for progress tracking endpoints.
// It reads the ID from the query parameter idParam, sets up SSE headers, and
// enters a ticker loop. Each tick it calls poll() which returns:
//   - data: the map to marshal as SSE JSON
//   - done: whether to stop after sending this event
//   - exists: whether the tracker was found
//
// While the tracker has not appeared and the wait time has not elapsed,
// placeholder is sent instead. After the wait time, an error event is sent.
func pollSSE(w http.ResponseWriter, r *http.Request, idParam string, placeholder map[string]any, errorData map[string]any, poll func() (data map[string]any, done bool, exists bool)) {
	id := r.URL.Query().Get(idParam)
	if id == "" {
		writeError(w, domain.ErrBadRequest(idParam+" is required"))
		return
	}

	flusher, ok := setupSSE(w)
	if !ok {
		writeError(w, domain.ErrInternal("streaming not supported"))
		return
	}

	ticker := time.NewTicker(service.SSEPollingInterval)
	defer ticker.Stop()
	waitStart := time.Now()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}

		data, done, exists := poll()
		if !exists {
			if time.Since(waitStart) < service.DownloadProgressWaitTime {
				writeSSE(w, flusher, placeholder)
				continue
			}
			writeSSE(w, flusher, errorData)
			return
		}

		writeSSE(w, flusher, data)
		if done {
			return
		}
	}
}

// sanitizeFilename returns a safe Content-Disposition filename value.
func sanitizeFilename(name string) string {
	return strings.NewReplacer(`"`, `'`, "\n", "", "\r", "").Replace(name)
}

func isInlineVideo(contentType, filename string) bool {
	if strings.HasPrefix(strings.ToLower(contentType), "video/") {
		return true
	}
	_, ok := inlineVideoContentTypesByExtension[strings.ToLower(filepath.Ext(filename))]
	return ok
}

func contentTypeForFilename(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if contentType := mime.TypeByExtension(ext); contentType != "" {
		return contentType
	}
	if contentType, ok := inlineVideoContentTypesByExtension[ext]; ok {
		return contentType
	}
	return "application/octet-stream"
}
