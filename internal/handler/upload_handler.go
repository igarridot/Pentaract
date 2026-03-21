package handler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/pathutil"
	"github.com/Dominux/Pentaract/internal/service"
)

func (h *FilesHandler) scheduleUploadTrackerCleanup(uploadID string) {
	time.AfterFunc(service.TrackerCleanupDelay, func() {
		h.mu.Lock()
		delete(h.uploads, uploadID)
		h.mu.Unlock()
	})
}

func (h *FilesHandler) Upload(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	storageID, err := parseUUIDParam(r, "storageID")
	if err != nil {
		writeError(w, err)
		return
	}

	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, domain.ErrBadRequest("multipart form required"))
		return
	}

	var path, filename, uploadID, onConflict string
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
		case "file":
			filename = part.FileName()
			filePart = part
			fileSize = r.ContentLength
			if fileSize < 0 {
				fileSize = 0
			}
		}
		if filePart != nil {
			break
		}
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

	pr, pw := io.Pipe()
	copyDone := make(chan struct{})

	go func() {
		defer close(copyDone)
		_, err := io.Copy(pw, filePart)
		filePart.Close()
		pw.CloseWithError(err)
	}()

	uploadCtx, cancel := context.WithCancel(context.Background())

	if uploadID == "" {
		uploadID = uuid.New().String()
	}
	progress := &service.UploadProgress{TotalBytes: fileSize}
	tracker := &uploadTracker{
		progress:  progress,
		cancel:    cancel,
		storageID: storageID,
		filePath:  fullPath,
	}

	h.mu.Lock()
	h.uploads[uploadID] = tracker
	h.mu.Unlock()

	go func() {
		defer func() {
			h.scheduleUploadTrackerCleanup(uploadID)
		}()

		_, skipped, uploadErr := h.svc.Upload(uploadCtx, user.ID, storageID, fullPath, fileSize, pr, progress, onConflict)

		h.mu.Lock()
		tracker.done = true
		tracker.err = uploadErr
		tracker.skipped = skipped
		h.mu.Unlock()

		if uploadErr != nil {
			slog.Error("upload failed", "file", fullPath, "err", uploadErr)
			pr.Close()
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{"upload_id": uploadID})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Keep the handler alive until the body is fully read into the pipe.
	<-copyDone
}

// CancelUpload cancels an in-flight upload and cleans up.
func (h *FilesHandler) CancelUpload(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	uploadID := chi.URLParam(r, "uploadID")

	h.mu.RLock()
	tracker, exists := h.uploads[uploadID]
	h.mu.RUnlock()

	if !exists {
		writeError(w, domain.ErrNotFound("upload"))
		return
	}

	tracker.cancel()

	slog.Info("cancelling upload", "upload_id", uploadID, "file", tracker.filePath)

	go func() {
		time.Sleep(1 * time.Second)
		if err := h.svc.Delete(context.Background(), user.ID, tracker.storageID, tracker.filePath, nil, false); err != nil {
			slog.Warn("upload cancel cleanup failed", "file", tracker.filePath, "err", err)
		} else {
			slog.Info("upload cancel cleanup done", "file", tracker.filePath)
		}
	}()

	w.WriteHeader(http.StatusNoContent)
}

// UploadProgress returns an SSE stream with upload progress updates.
func (h *FilesHandler) UploadProgress(w http.ResponseWriter, r *http.Request) {
	uploadID := r.URL.Query().Get("upload_id")

	pollSSE(w, r, "upload_id",
		map[string]any{
			"total": 0, "uploaded": 0, "total_bytes": 0, "uploaded_bytes": 0,
			"verification_total": 0, "verified": 0, "status": "uploading",
		},
		map[string]any{"status": "error"},
		func() (map[string]any, bool, bool) {
			h.mu.RLock()
			tracker, exists := h.uploads[uploadID]
			h.mu.RUnlock()
			if !exists {
				return nil, false, false
			}

			p := tracker.progress
			h.mu.RLock()
			isDone := tracker.done
			uploadErr := tracker.err
			isSkipped := tracker.skipped
			h.mu.RUnlock()
			status := uploadProgressStatus(p, isDone, uploadErr, isSkipped)

			return map[string]any{
				"total":              p.TotalChunks,
				"uploaded":           p.UploadedChunks.Load(),
				"total_bytes":        p.TotalBytes,
				"uploaded_bytes":     p.UploadedBytes.Load(),
				"verification_total": p.VerificationTotalChunks,
				"verified":           p.VerifiedChunks.Load(),
				"status":             status,
				"workers_status":     h.svc.WorkersStatus(tracker.storageID),
			}, isDone, true
		},
	)
}
