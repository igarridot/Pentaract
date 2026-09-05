package handler

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/pathutil"
	"github.com/Dominux/Pentaract/internal/service"
)

// LocalUploadMountPath is the fixed mount point inside the container where
// the host directory specified by LOCAL_UPLOAD_BASE_PATH is mounted.
const LocalUploadMountPath = "/mnt/data"

// UploadHandler receives browser uploads and local-mount uploads, tracks
// their progress and answers the upload SSE stream.
type UploadHandler struct {
	svc uploadService

	uploadsMu sync.RWMutex
	uploads   map[string]*uploadTracker

	// localBasePath is set when the local upload mount exists; empty disables
	// the local upload endpoints.
	localBasePath string
}

func NewUploadHandler(svc uploadService) *UploadHandler {
	basePath := ""
	if info, err := os.Stat(LocalUploadMountPath); err == nil && info.IsDir() {
		basePath = LocalUploadMountPath
	}
	return &UploadHandler{
		svc:           svc,
		uploads:       make(map[string]*uploadTracker),
		localBasePath: basePath,
	}
}

func (h *UploadHandler) scheduleUploadTrackerCleanup(uploadID string) {
	time.AfterFunc(service.TrackerCleanupDelay, func() {
		h.uploadsMu.Lock()
		delete(h.uploads, uploadID)
		h.uploadsMu.Unlock()
	})
}

// cancelUploadSettleDelay is how long CancelUpload waits for the upload
// goroutine to record the file id before cleaning up. A variable for tests.
var cancelUploadSettleDelay = time.Second

// registerUpload creates and registers the tracker for an upload so its
// upload_id can be subscribed to immediately, before any byte is transferred.
// The returned context is cancelled by CancelUpload.
func (h *UploadHandler) registerUpload(uploadID string, storageID uuid.UUID, fullPath string, fileSize int64) (*uploadTracker, context.Context) {
	uploadCtx, cancel := context.WithCancel(context.Background())
	tracker := &uploadTracker{
		progress:  &service.UploadProgress{TotalBytes: fileSize},
		cancel:    cancel,
		storageID: storageID,
		filePath:  fullPath,
	}
	h.uploadsMu.Lock()
	h.uploads[uploadID] = tracker
	h.uploadsMu.Unlock()
	return tracker, uploadCtx
}

// runTrackedUpload feeds src through a one-chunk buffer into the upload
// service and records the outcome on the tracker. Buffering one chunk ahead
// lets the source reader race ahead of the encryption/upload pipeline, which
// smooths throughput. src is always closed.
func (h *UploadHandler) runTrackedUpload(ctx context.Context, tracker *uploadTracker, userID, storageID uuid.UUID, fullPath string, fileSize int64, src io.ReadCloser, onConflict string) {
	pr, pw := io.Pipe()
	go func() {
		bw := bufio.NewWriterSize(pw, service.UploadChunkSize)
		_, err := io.Copy(bw, src)
		if flushErr := bw.Flush(); err == nil {
			err = flushErr
		}
		src.Close()
		if err != nil {
			// A source that fails mid-stream (browser gone, multipart body cut
			// short) must not look like a plain short read: io.ErrUnexpectedEOF
			// is how the uploader recognises the legitimate last chunk.
			err = fmt.Errorf("%w: %v", domain.ErrUploadInterrupted, err)
		}
		pw.CloseWithError(err)
	}()
	// Always close the pipe reader so the copy goroutine unblocks, even when
	// the upload was skipped or finished without consuming the full stream.
	defer pr.Close()

	file, skipped, uploadErr := h.svc.Upload(ctx, userID, storageID, fullPath, fileSize, pr, tracker.progress, onConflict)
	h.finishUpload(tracker, file, skipped, uploadErr)
}

func (h *UploadHandler) finishUpload(tracker *uploadTracker, file *domain.File, skipped bool, uploadErr error) {
	h.uploadsMu.Lock()
	tracker.done = true
	tracker.err = uploadErr
	tracker.skipped = skipped
	if file != nil {
		tracker.fileID = file.ID
	}
	h.uploadsMu.Unlock()

	if uploadErr != nil {
		slog.Error("upload failed", "file", tracker.filePath, "err", uploadErr)
	}
}

func (h *UploadHandler) Upload(w http.ResponseWriter, r *http.Request) {
	user, storageID, err := storageRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, domain.ErrBadRequest("multipart form required"))
		return
	}

	var path, filename, uploadID, onConflict, fileSizeStr string
	var fileSize int64
	var filePart io.ReadCloser

	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		switch part.FormName() {
		case "path":
			b, _ := io.ReadAll(part)
			path = string(b)
		case "upload_id":
			b, _ := io.ReadAll(part)
			uploadID = string(b)
		case "on_conflict":
			b, _ := io.ReadAll(part)
			onConflict = string(b)
		case "file_size":
			b, _ := io.ReadAll(part)
			fileSizeStr = string(b)
		case "file":
			filename = part.FileName()
			filePart = part
		}
		if filePart != nil {
			break
		}
	}

	if fileSizeStr != "" {
		if parsed, err := strconv.ParseInt(fileSizeStr, 10, 64); err == nil && parsed > 0 {
			fileSize = parsed
		}
	}
	if fileSize <= 0 {
		fileSize = max(r.ContentLength, 0)
	}

	if filePart == nil || filename == "" {
		writeError(w, domain.ErrBadRequest("file is required"))
		return
	}

	path = pathutil.TrimTrailingSlash(path)
	if onConflict == "" {
		onConflict = service.UploadConflictKeepBoth
	}
	fullPath := pathutil.Join(path, filename)
	if uploadID == "" {
		uploadID = uuid.New().String()
	}

	tracker, uploadCtx := h.registerUpload(uploadID, storageID, fullPath, fileSize)

	// The multipart part is only readable while this handler runs, so the
	// upload goroutine signals when it has consumed the whole body. The
	// response must wait for that signal: when a handler starts writing while
	// the request body is unread, net/http discards the rest of a body under
	// 256 KB (maxPostHandlerReadBytes), so answering early truncated every
	// small upload to the few KB the multipart parser had already buffered.
	bodyConsumed := make(chan struct{})
	go func() {
		defer h.scheduleUploadTrackerCleanup(uploadID)
		h.runTrackedUpload(uploadCtx, tracker, user.ID, storageID, fullPath, fileSize, &signalOnClose{ReadCloser: filePart, done: bodyConsumed}, onConflict)
	}()

	<-bodyConsumed
	writeJSON(w, http.StatusAccepted, map[string]any{"upload_id": uploadID})
}

// signalOnClose closes done once the wrapped reader is closed.
type signalOnClose struct {
	io.ReadCloser
	done chan struct{}
	once sync.Once
}

func (s *signalOnClose) Close() error {
	err := s.ReadCloser.Close()
	s.once.Do(func() { close(s.done) })
	return err
}

// CancelUpload cancels an in-flight upload and cleans up.
func (h *UploadHandler) CancelUpload(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	uploadID := chi.URLParam(r, "uploadID")

	h.uploadsMu.RLock()
	tracker, exists := h.uploads[uploadID]
	h.uploadsMu.RUnlock()

	if !exists {
		writeError(w, domain.ErrNotFound("upload"))
		return
	}

	tracker.cancel()

	slog.Info("cancelling upload", "upload_id", uploadID, "file", tracker.filePath)

	go func() {
		// Wait for the upload goroutine to finish so tracker.fileID is set.
		time.Sleep(cancelUploadSettleDelay)

		h.uploadsMu.RLock()
		fileID := tracker.fileID
		h.uploadsMu.RUnlock()

		if fileID == uuid.Nil {
			// File record was never created (cancelled before DB insert).
			slog.Info("upload cancel: no file record to clean up", "file", tracker.filePath)
			return
		}

		if err := h.svc.CleanupCancelledUpload(context.Background(), user.ID, tracker.storageID, fileID); err != nil {
			slog.Warn("upload cancel cleanup failed", "file", tracker.filePath, "err", err)
		} else {
			slog.Info("upload cancel cleanup done", "file", tracker.filePath)
		}
	}()

	w.WriteHeader(http.StatusNoContent)
}

// UploadProgress returns an SSE stream with upload progress updates.
func (h *UploadHandler) UploadProgress(w http.ResponseWriter, r *http.Request) {
	uploadID := r.URL.Query().Get("upload_id")

	pollSSE(w, r, "upload_id",
		map[string]any{
			"total": 0, "uploaded": 0, "total_bytes": 0, "uploaded_bytes": 0,
			"verification_total": 0, "verified": 0, "status": "uploading",
		},
		map[string]any{"status": "error"},
		func() (map[string]any, bool, bool) {
			h.uploadsMu.RLock()
			tracker, exists := h.uploads[uploadID]
			h.uploadsMu.RUnlock()
			if !exists {
				return nil, false, false
			}

			p := tracker.progress
			h.uploadsMu.RLock()
			isDone := tracker.done
			uploadErr := tracker.err
			isSkipped := tracker.skipped
			h.uploadsMu.RUnlock()
			status := uploadProgressStatus(p, isDone, uploadErr, isSkipped)

			return map[string]any{
				"total":              p.TotalChunks,
				"uploaded":           p.UploadedChunks.Load(),
				"total_bytes":        p.TotalBytes,
				"uploaded_bytes":     p.UploadedBytes.Load(),
				"verification_total": p.VerificationTotalChunks.Load(),
				"verified":           p.VerifiedChunks.Load(),
				"status":             status,
				"workers_status":     h.svc.WorkersStatus(tracker.storageID),
			}, isDone, true
		},
	)
}
