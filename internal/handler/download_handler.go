package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/pathutil"
	"github.com/Dominux/Pentaract/internal/service"
)

// DownloadHandler streams files and directory archives, tracks download
// progress for the UI and answers the download SSE stream.
type DownloadHandler struct {
	svc downloadService

	downloadsMu sync.RWMutex
	downloads   map[string]*downloadTracker

	fileSizes *fileSizeCache
}

func NewDownloadHandler(svc downloadService) *DownloadHandler {
	return &DownloadHandler{
		svc:       svc,
		downloads: make(map[string]*downloadTracker),
		fileSizes: newFileSizeCache(),
	}
}

// setupDownloadTracker creates a download tracker from the request's download_id param.
// Returns the context to use, the tracker (may be nil), and a cleanup function to defer.
func (h *DownloadHandler) setupDownloadTracker(r *http.Request, storageID uuid.UUID) (context.Context, *downloadTracker, func()) {
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
	h.downloadsMu.Lock()
	if previous, ok := h.downloads[downloadID]; ok {
		previous.canceled = true
		previous.done = true
		if previous.cancel != nil {
			previous.cancel()
		}
	}
	h.downloads[downloadID] = tracker
	h.downloadsMu.Unlock()
	return ctx, tracker, func() {
		h.scheduleDownloadTrackerCleanup(downloadID, tracker)
	}
}

// downloadInterruptedGracePeriod is a variable so tests can shorten the wait.
var downloadInterruptedGracePeriod = service.DownloadInterruptedGracePeriod

func (h *DownloadHandler) cleanupDownloadTracker(downloadID string, tracker *downloadTracker) {
	h.downloadsMu.Lock()
	defer h.downloadsMu.Unlock()
	current, ok := h.downloads[downloadID]
	if !ok || current != tracker {
		return
	}
	// An interrupted tracker is still waiting for the browser to resume the
	// download; the grace timer started by interruptTracker owns its cleanup.
	if tracker.interrupted && !tracker.done {
		return
	}
	delete(h.downloads, downloadID)
}

func (h *DownloadHandler) scheduleDownloadTrackerCleanup(downloadID string, tracker *downloadTracker) {
	time.AfterFunc(service.TrackerCleanupDelay, func() {
		h.cleanupDownloadTracker(downloadID, tracker)
	})
}

// finishTracker marks a download tracker as done with an optional error.
func (h *DownloadHandler) finishTracker(tracker *downloadTracker, err error) {
	if tracker == nil {
		return
	}
	h.downloadsMu.Lock()
	tracker.done = true
	tracker.err = err
	h.downloadsMu.Unlock()
}

// failTracker records a failed download request. When the failure is just the
// browser dropping the connection (Chrome, for instance, interrupts downloads
// of uncommon file types served over plain HTTP and only re-requests them once
// the user allows the download from its download list), the tracker is kept
// alive as "interrupted" instead of being reported as an error, so the resumed
// request with the same download_id can carry on with the same progress stream.
func (h *DownloadHandler) failTracker(r *http.Request, tracker *downloadTracker, err error) {
	if tracker == nil {
		return
	}
	h.downloadsMu.RLock()
	canceled := tracker.canceled
	h.downloadsMu.RUnlock()
	if !canceled && clientDisconnected(r, err) {
		h.interruptTracker(r.URL.Query().Get("download_id"), tracker)
		return
	}
	h.finishTracker(tracker, err)
}

// interruptTracker parks the tracker until the browser resumes the download or
// the grace period runs out. On expiry the tracker is failed with
// domain.ErrClientDisconnected and scheduled for removal, unless a newer
// request already replaced it.
func (h *DownloadHandler) interruptTracker(downloadID string, tracker *downloadTracker) {
	h.downloadsMu.Lock()
	tracker.interrupted = true
	h.downloadsMu.Unlock()

	time.AfterFunc(downloadInterruptedGracePeriod, func() {
		h.downloadsMu.Lock()
		current := h.downloads[downloadID] == tracker
		if current && !tracker.done {
			tracker.done = true
			tracker.err = domain.ErrClientDisconnected
		}
		h.downloadsMu.Unlock()
		if current {
			h.scheduleDownloadTrackerCleanup(downloadID, tracker)
		}
	})
}

// clientDisconnected reports whether a failed download ended because the
// browser closed the connection rather than because the transfer itself failed.
func clientDisconnected(r *http.Request, err error) bool {
	if r.Context().Err() != nil {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		// Upstream (Telegram) transport failure, not our client.
		return false
	}
	var opErr *net.OpError
	return errors.As(err, &opErr) && opErr.Op == "write"
}

// exactFileSize returns the byte size announced in Content-Length and Range
// responses. It is measured from the chunks (cached per file) rather than read
// from the record: sizes stored by older clients could come from the request's
// Content-Length, which includes multipart framing, and an inexact
// Content-Length makes browsers hang or truncate the download.
func (h *DownloadHandler) exactFileSize(ctx context.Context, file *domain.File) (int64, error) {
	if totalSize, ok := h.fileSizes.get(file.ID); ok {
		return totalSize, nil
	}

	totalSize, err := h.svc.ExactFileSize(ctx, file)
	if err != nil {
		return 0, err
	}

	h.fileSizes.set(file.ID, totalSize)
	return totalSize, nil
}

func (h *DownloadHandler) Download(w http.ResponseWriter, r *http.Request) {
	user, storageID, err := storageRequest(r)
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

	served := servedFile{
		file:        file,
		filename:    filepath.Base(file.Path),
		contentType: contentTypeForFilename(filepath.Base(file.Path)),
	}
	if r.URL.Query().Get("inline") == "1" {
		h.serveInline(w, r, served)
		return
	}
	h.serveAttachment(w, r, served, storageID)
}

// servedFile is what the response describes: the record plus the derived
// filename and content type.
type servedFile struct {
	file        *domain.File
	filename    string
	contentType string
}

func (s servedFile) writeHeaders(w http.ResponseWriter, disposition string) {
	w.Header().Set("Content-Type", s.contentType)
	w.Header().Set("Content-Disposition", disposition+`; filename="`+sanitizeFilename(s.filename)+`"`)
}

// trackedDownload sets up progress tracking for downloads the UI follows via
// download_id. The returned cleanup must be deferred by the caller.
func (h *DownloadHandler) trackedDownload(r *http.Request, storageID uuid.UUID) (context.Context, *downloadTracker, *service.DownloadProgress, func()) {
	ctx, tracker, cleanup := h.setupDownloadTracker(r, storageID)
	var progress *service.DownloadProgress
	if tracker != nil {
		progress = tracker.progress
	}
	return ctx, tracker, progress, cleanup
}

// serveInline serves a file for in-browser preview or playback, honouring
// Range requests by fetching only the chunks that cover the window. Playback
// fans out into several concurrent range requests that share one download_id,
// so inline requests are deliberately not tracked: tracking them would make
// newer range requests cancel older ones.
func (h *DownloadHandler) serveInline(w http.ResponseWriter, r *http.Request, s servedFile) {
	ctx := r.Context()
	s.writeHeaders(w, "inline")
	w.Header().Set("Accept-Ranges", "bytes")

	totalSize, err := h.exactFileSize(ctx, s.file)
	if err != nil {
		slog.Error("file size resolution failed", "err", err)
		writeError(w, domain.ErrInternal("failed to determine exact file size"))
		return
	}

	rangeHeader := r.Header.Get("Range")
	if rangeHeader == "" {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", totalSize))
		w.WriteHeader(http.StatusOK)
		if err := h.svc.StreamFileToWriter(ctx, s.file, w, nil); err != nil {
			slog.Error("stream file failed", "err", err)
		}
		return
	}

	start, end, ok := writeRangeHeaders(w, rangeHeader, totalSize)
	if !ok {
		return
	}
	if err := h.svc.DownloadFileRangeToWriter(ctx, s.file, w, start, end, totalSize, nil); err != nil {
		slog.Error("download file range failed", "err", err)
	}
}

// writeRangeHeaders answers a single byte-range request: 206 with the range
// headers when it is satisfiable, 416 otherwise (ok=false, nothing left to do).
func writeRangeHeaders(w http.ResponseWriter, rangeHeader string, totalSize int64) (start, end int64, ok bool) {
	start, end, err := parseSingleByteRange(rangeHeader, totalSize)
	if err != nil {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", totalSize))
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return 0, 0, false
	}
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, totalSize))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", end-start+1))
	w.WriteHeader(http.StatusPartialContent)
	return start, end, true
}

// serveAttachment streams the file as a download. It always announces the
// exact Content-Length and honours Range requests: a browser that interrupts
// the download (Chrome blocking an "insecure" file type until the user allows
// it) can then tell the file is incomplete and resume from where it stopped
// instead of keeping the truncated bytes as the final file.
func (h *DownloadHandler) serveAttachment(w http.ResponseWriter, r *http.Request, s servedFile, storageID uuid.UUID) {
	ctx, tracker, progress, cleanup := h.trackedDownload(r, storageID)
	defer cleanup()

	totalSize, err := h.exactFileSize(ctx, s.file)
	if err != nil {
		slog.Error("file size resolution failed", "err", err)
		h.failTracker(r, tracker, err)
		writeError(w, domain.ErrInternal("failed to determine exact file size"))
		return
	}

	s.writeHeaders(w, "attachment")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("ETag", fmt.Sprintf(`"%s-%d"`, s.file.ID, totalSize))

	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		start, end, ok := writeRangeHeaders(w, rangeHeader, totalSize)
		if !ok {
			h.finishTracker(tracker, nil)
			return
		}
		if progress != nil {
			progress.TotalBytes = end - start + 1
		}
		err = h.svc.DownloadFileRangeToWriter(ctx, s.file, w, start, end, totalSize, progress)
	} else {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", totalSize))
		w.WriteHeader(http.StatusOK)
		err = h.svc.DownloadFileToWriter(ctx, s.file, w, progress)
	}
	if err != nil {
		slog.Error("download file failed", "err", err)
		h.failTracker(r, tracker, err)
		return
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

func (h *DownloadHandler) DownloadDir(w http.ResponseWriter, r *http.Request) {
	user, storageID, err := storageRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	path := extractWildcardPath(r)

	dirName := pathutil.ArchiveName(path)

	downloadCtx, tracker, progress, cleanup := h.trackedDownload(r, storageID)
	defer cleanup()

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, sanitizeFilename(dirName)))

	writer := io.Writer(w)
	if flusher, ok := w.(http.Flusher); ok {
		writer = &flushWriter{w: w, flusher: flusher}
		flusher.Flush()
	}

	if _, err := h.svc.DownloadDir(downloadCtx, user.ID, storageID, path, writer, progress); err != nil {
		slog.Error("download dir failed", "err", err)
		h.failTracker(r, tracker, err)
		return
	}

	h.finishTracker(tracker, nil)
}

func (h *DownloadHandler) CancelDownload(w http.ResponseWriter, r *http.Request) {
	downloadID := chi.URLParam(r, "downloadID")

	h.downloadsMu.Lock()
	tracker, exists := h.downloads[downloadID]
	if !exists {
		h.downloadsMu.Unlock()
		writeError(w, domain.ErrNotFound("download"))
		return
	}
	tracker.canceled = true
	tracker.done = true
	if tracker.cancel != nil {
		tracker.cancel()
	}
	h.downloadsMu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

// DownloadProgress returns an SSE stream with directory download progress updates.
func (h *DownloadHandler) DownloadProgress(w http.ResponseWriter, r *http.Request) {
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
			h.downloadsMu.RLock()
			tracker, exists := h.downloads[downloadID]
			h.downloadsMu.RUnlock()
			if !exists {
				return nil, false, false
			}

			p := tracker.progress
			status := "downloading"

			h.downloadsMu.RLock()
			isDone := tracker.done
			downloadErr := tracker.err
			isCanceled := tracker.canceled
			isInterrupted := tracker.interrupted
			h.downloadsMu.RUnlock()

			if isDone && isCanceled {
				status = "cancelled"
			} else if isDone && downloadErr != nil {
				status = "error"
			} else if isDone {
				status = "done"
			} else if isInterrupted {
				status = "interrupted"
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
