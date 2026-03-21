package handler

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/pathutil"
	"github.com/Dominux/Pentaract/internal/service"
)

// setupDownloadTracker creates a download tracker from the request's download_id param.
// Returns the context to use, the tracker (may be nil), and a cleanup function to defer.
func (h *FilesHandler) setupDownloadTracker(r *http.Request, storageID uuid.UUID) (context.Context, *downloadTracker, func()) {
	downloadID := r.URL.Query().Get("download_id")
	if downloadID == "" {
		return r.Context(), nil, func() {}
	}
	ctx, cancel := context.WithCancel(r.Context())
	tracker := &downloadTracker{
		storageID: storageID,
		progress:  &service.DownloadProgress{},
		cancel:    cancel,
	}
	h.mu.Lock()
	if previous, ok := h.downloads[downloadID]; ok {
		previous.canceled = true
		previous.done = true
		if previous.cancel != nil {
			previous.cancel()
		}
	}
	h.downloads[downloadID] = tracker
	h.mu.Unlock()
	return ctx, tracker, func() {
		h.scheduleDownloadTrackerCleanup(downloadID, tracker)
	}
}

func (h *FilesHandler) cleanupDownloadTracker(downloadID string, tracker *downloadTracker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if current, ok := h.downloads[downloadID]; ok && current == tracker {
		delete(h.downloads, downloadID)
	}
}

func (h *FilesHandler) scheduleDownloadTrackerCleanup(downloadID string, tracker *downloadTracker) {
	time.AfterFunc(service.TrackerCleanupDelay, func() {
		h.cleanupDownloadTracker(downloadID, tracker)
	})
}

// finishTracker marks a download tracker as done with an optional error.
func (h *FilesHandler) finishTracker(tracker *downloadTracker, err error) {
	if tracker == nil {
		return
	}
	h.mu.Lock()
	tracker.done = true
	tracker.err = err
	h.mu.Unlock()
}

func (h *FilesHandler) resolvedInlineVideoSize(ctx context.Context, file *domain.File) (int64, error) {
	if totalSize, ok := h.fileSizes.get(file.ID); ok {
		return totalSize, nil
	}
	if file.Size > 0 {
		h.fileSizes.set(file.ID, file.Size)
		return file.Size, nil
	}

	totalSize, err := h.svc.ExactFileSize(ctx, file)
	if err != nil {
		return 0, err
	}

	h.fileSizes.set(file.ID, totalSize)
	return totalSize, nil
}

func (h *FilesHandler) Download(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	storageID, err := parseUUIDParam(r, "storageID")
	if err != nil {
		writeError(w, err)
		return
	}

	path := extractWildcardPath(r)
	if path == "" {
		writeError(w, domain.ErrBadRequest("file path is required"))
		return
	}

	file, err := h.svc.GetFileForDownload(r.Context(), user.ID, storageID, path)
	if err != nil {
		writeError(w, err)
		return
	}

	downloadCtx, tracker, cleanup := h.setupDownloadTracker(r, storageID)
	defer cleanup()

	filename := filepath.Base(file.Path)
	contentType := contentTypeForFilename(filename)
	disposition := "attachment"
	if r.URL.Query().Get("inline") == "1" {
		disposition = "inline"
	}

	var progress *service.DownloadProgress
	if tracker != nil {
		progress = tracker.progress
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", disposition+`; filename="`+sanitizeFilename(filename)+`"`)

	switch {
	case disposition == "inline" && isInlineVideo(contentType, filename):
		// Video: handle Range requests for streaming.
		w.Header().Set("Accept-Ranges", "bytes")
		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" {
			totalSize, sizeErr := h.resolvedInlineVideoSize(downloadCtx, file)
			if sizeErr != nil {
				slog.Error("video size resolution failed", "err", sizeErr)
				writeError(w, domain.ErrInternal("failed to determine exact video size"))
				return
			}
			start, end, err := parseSingleByteRange(rangeHeader, totalSize)
			if err != nil {
				w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", totalSize))
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, totalSize))
			w.Header().Set("Content-Length", fmt.Sprintf("%d", end-start+1))
			w.WriteHeader(http.StatusPartialContent)
			if err := h.svc.DownloadFileRangeToWriter(downloadCtx, file, w, start, end, totalSize, progress); err != nil {
				slog.Error("download file range failed", "err", err)
				h.finishTracker(tracker, err)
				return
			}
		} else {
			if file.Size > 0 {
				w.Header().Set("Content-Length", fmt.Sprintf("%d", file.Size))
			}
			w.WriteHeader(http.StatusOK)
			if err := h.svc.StreamFileToWriter(downloadCtx, file, w, progress); err != nil {
				slog.Error("stream file failed", "err", err)
				h.finishTracker(tracker, err)
				return
			}
		}

	case disposition == "inline":
		// Non-video inline: download to temp file, serve with http.ServeContent.
		tmp, err := os.CreateTemp("", "pentaract-file-*")
		if err != nil {
			writeError(w, domain.ErrInternal("failed to create temporary file"))
			return
		}
		tmpPath := tmp.Name()
		defer func() {
			tmp.Close()
			_ = os.Remove(tmpPath)
		}()

		if err := h.svc.DownloadFileToWriter(downloadCtx, file, tmp, progress); err != nil {
			slog.Error("download file failed", "err", err)
			h.finishTracker(tracker, err)
			writeError(w, err)
			return
		}

		info, err := tmp.Stat()
		if err != nil {
			writeError(w, domain.ErrInternal("failed to stat temporary file"))
			return
		}
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			writeError(w, domain.ErrInternal("failed to rewind temporary file"))
			return
		}
		w.Header().Set("Accept-Ranges", "bytes")
		http.ServeContent(w, r, filename, info.ModTime(), tmp)

	default:
		// Attachment: stream directly.
		w.WriteHeader(http.StatusOK)
		if err := h.svc.DownloadFileToWriter(downloadCtx, file, w, progress); err != nil {
			slog.Error("download file failed", "err", err)
			h.finishTracker(tracker, err)
			return
		}
	}

	h.finishTracker(tracker, nil)
}

func parseSingleByteRange(header string, size int64) (int64, int64, error) {
	if !strings.HasPrefix(header, "bytes=") {
		return 0, 0, fmt.Errorf("invalid range unit")
	}
	rangeSpec := strings.TrimSpace(strings.TrimPrefix(header, "bytes="))
	if rangeSpec == "" || strings.Contains(rangeSpec, ",") {
		return 0, 0, fmt.Errorf("multiple/empty ranges not supported")
	}
	parts := strings.SplitN(rangeSpec, "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range format")
	}

	// Suffix range: bytes=-N
	if parts[0] == "" {
		suffix, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, fmt.Errorf("invalid suffix range")
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, size - 1, nil
	}

	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, fmt.Errorf("invalid range start")
	}

	// Open range: bytes=N-
	if parts[1] == "" {
		return start, size - 1, nil
	}

	end, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || end < start {
		return 0, 0, fmt.Errorf("invalid range end")
	}
	if end >= size {
		end = size - 1
	}
	return start, end, nil
}

func (h *FilesHandler) DownloadDir(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	storageID, err := parseUUIDParam(r, "storageID")
	if err != nil {
		writeError(w, err)
		return
	}

	path := extractWildcardPath(r)

	dirName := pathutil.ArchiveName(path)

	downloadCtx, tracker, cleanup := h.setupDownloadTracker(r, storageID)
	defer cleanup()

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, sanitizeFilename(dirName)))

	var progress *service.DownloadProgress
	if tracker != nil {
		progress = tracker.progress
	}

	writer := io.Writer(w)
	if flusher, ok := w.(http.Flusher); ok {
		writer = &flushWriter{w: w, flusher: flusher}
		flusher.Flush()
	}

	if _, err := h.svc.DownloadDir(downloadCtx, user.ID, storageID, path, writer, progress); err != nil {
		slog.Error("download dir failed", "err", err)
		h.finishTracker(tracker, err)
		return
	}

	h.finishTracker(tracker, nil)
}

func (h *FilesHandler) CancelDownload(w http.ResponseWriter, r *http.Request) {
	downloadID := chi.URLParam(r, "downloadID")

	h.mu.Lock()
	tracker, exists := h.downloads[downloadID]
	if !exists {
		h.mu.Unlock()
		writeError(w, domain.ErrNotFound("download"))
		return
	}
	tracker.canceled = true
	tracker.done = true
	if tracker.cancel != nil {
		tracker.cancel()
	}
	h.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

// DownloadProgress returns an SSE stream with directory download progress updates.
func (h *FilesHandler) DownloadProgress(w http.ResponseWriter, r *http.Request) {
	downloadID := r.URL.Query().Get("download_id")

	pollSSE(w, r, "download_id",
		map[string]any{
			"total": 0, "downloaded": 0, "total_bytes": 0, "downloaded_bytes": 0, "status": "downloading",
		},
		map[string]any{
			"status":        "error",
			"error_message": "Download did not start on the server. Please try again.",
		},
		func() (map[string]any, bool, bool) {
			h.mu.RLock()
			tracker, exists := h.downloads[downloadID]
			h.mu.RUnlock()
			if !exists {
				return nil, false, false
			}

			p := tracker.progress
			status := "downloading"

			h.mu.RLock()
			isDone := tracker.done
			downloadErr := tracker.err
			isCanceled := tracker.canceled
			h.mu.RUnlock()

			if isDone && isCanceled {
				status = "cancelled"
			} else if isDone && downloadErr != nil {
				status = "error"
			} else if isDone {
				status = "done"
			}

			return map[string]any{
				"total":            p.TotalChunks,
				"downloaded":       p.DownloadedChunks.Load(),
				"total_bytes":      p.TotalBytes,
				"downloaded_bytes": p.DownloadedBytes.Load(),
				"status":           status,
				"workers_status":   h.svc.WorkersStatus(tracker.storageID),
				"error_message":    downloadErrorMessage(downloadErr),
			}, isDone, true
		},
	)
}

// DeleteProgress returns an SSE stream with delete progress updates.
func (h *FilesHandler) DeleteProgress(w http.ResponseWriter, r *http.Request) {
	deleteID := r.URL.Query().Get("delete_id")

	pollSSE(w, r, "delete_id",
		map[string]any{
			"total": 0, "deleted": 0, "pending": 0, "status": "deleting",
		},
		map[string]any{"status": "error"},
		func() (map[string]any, bool, bool) {
			tracker, exists := getDeleteTracker(deleteID)
			if !exists {
				return nil, false, false
			}

			done, trackerErr, total, deleted := getDeleteTrackerStatus(tracker)
			status := "deleting"
			if done && trackerErr != nil {
				status = "error"
			} else if done {
				status = "done"
			}

			pending := total - deleted
			if pending < 0 {
				pending = 0
			}

			return map[string]any{
				"total":          total,
				"deleted":        deleted,
				"pending":        pending,
				"status":         status,
				"workers_status": h.svc.WorkersStatus(tracker.storageID),
			}, done, true
		},
	)
}
