package handler

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/service"
)

type uploadTracker struct {
	progress  *service.UploadProgress
	cancel    context.CancelFunc
	storageID uuid.UUID
	filePath  string
	err       error // set when upload finishes with error
	done      bool  // set when upload finishes
}

type FilesHandler struct {
	svc *service.FilesService

	mu      sync.RWMutex
	uploads map[string]*uploadTracker
}

func NewFilesHandler(svc *service.FilesService) *FilesHandler {
	return &FilesHandler{
		svc:     svc,
		uploads: make(map[string]*uploadTracker),
	}
}

type createFolderRequest struct {
	Path       string `json:"path"`
	FolderName string `json:"folder_name"`
}

func (h *FilesHandler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	storageID, err := parseUUIDParam(r, "storageID")
	if err != nil {
		writeError(w, err)
		return
	}

	var req createFolderRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, err)
		return
	}

	if req.FolderName == "" {
		writeError(w, domain.ErrBadRequest("folder_name is required"))
		return
	}

	if err := h.svc.CreateFolder(r.Context(), user.ID, storageID, req.Path, req.FolderName); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
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

	var path, filename, uploadID string
	var fileSize int64
	var filePart io.ReadCloser

	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		switch part.FormName() {
		case "path":
			buf := make([]byte, 4096)
			n, _ := part.Read(buf)
			path = string(buf[:n])
		case "upload_id":
			buf := make([]byte, 256)
			n, _ := part.Read(buf)
			uploadID = string(buf[:n])
		case "file":
			filename = part.FileName()
			filePart = part
			fileSize = r.ContentLength
			if fileSize < 0 {
				fileSize = 0
			}
		}
		if filePart != nil && filename != "" {
			break
		}
	}

	if filePart == nil || filename == "" {
		writeError(w, domain.ErrBadRequest("file is required"))
		return
	}

	fullPath := filename
	if path != "" {
		fullPath = path + "/" + filename
	}

	// Pipe the multipart body to the upload goroutine.
	pr, pw := io.Pipe()

	// Channel to signal when body copy is done.
	copyDone := make(chan struct{})

	// Copy the multipart part data into the pipe in a goroutine.
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

	// Start upload in background
	go func() {
		defer func() {
			time.AfterFunc(30*time.Second, func() {
				h.mu.Lock()
				delete(h.uploads, uploadID)
				h.mu.Unlock()
			})
		}()

		_, uploadErr := h.svc.Upload(uploadCtx, user.ID, storageID, fullPath, fileSize, pr, progress)

		h.mu.Lock()
		tracker.done = true
		tracker.err = uploadErr
		h.mu.Unlock()

		if uploadErr != nil {
			log.Printf("[upload] failed file=%s: %v", fullPath, uploadErr)
			pr.Close()
		}
	}()

	// Respond immediately with upload_id so the client can track progress.
	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"upload_id": uploadID,
	})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Keep the handler alive until the body is fully read into the pipe.
	// This prevents Go's HTTP server from closing r.Body prematurely.
	<-copyDone
}

func (h *FilesHandler) UploadTo(w http.ResponseWriter, r *http.Request) {
	h.Upload(w, r)
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

	log.Printf("[cancel] cancelling upload %s file=%s", uploadID, tracker.filePath)

	go func() {
		// Wait briefly for the upload goroutine to stop
		time.Sleep(1 * time.Second)
		if err := h.svc.Delete(context.Background(), user.ID, tracker.storageID, tracker.filePath); err != nil {
			log.Printf("[cancel] WARNING: cleanup failed for %s: %v", tracker.filePath, err)
		} else {
			log.Printf("[cancel] cleanup done for %s", tracker.filePath)
		}
	}()

	w.WriteHeader(http.StatusNoContent)
}

// UploadProgress returns an SSE stream with upload progress updates.
func (h *FilesHandler) UploadProgress(w http.ResponseWriter, r *http.Request) {
	uploadID := r.URL.Query().Get("upload_id")
	if uploadID == "" {
		writeError(w, domain.ErrBadRequest("upload_id is required"))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, domain.ErrInternal("streaming not supported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(500 * time.Millisecond):
		}

		h.mu.RLock()
		tracker, exists := h.uploads[uploadID]
		h.mu.RUnlock()

		if !exists {
			fmt.Fprintf(w, "data: {\"status\":\"done\"}\n\n")
			flusher.Flush()
			return
		}

		p := tracker.progress
		total := p.TotalChunks
		uploaded := p.UploadedChunks.Load()
		totalBytes := p.TotalBytes
		uploadedBytes := p.UploadedBytes.Load()

		h.mu.RLock()
		isDone := tracker.done
		uploadErr := tracker.err
		h.mu.RUnlock()

		if isDone && uploadErr != nil {
			fmt.Fprintf(w, "data: {\"total\":%d,\"uploaded\":%d,\"total_bytes\":%d,\"uploaded_bytes\":%d,\"status\":\"error\"}\n\n",
				total, uploaded, totalBytes, uploadedBytes)
			flusher.Flush()
			return
		}

		if isDone && uploadErr == nil {
			fmt.Fprintf(w, "data: {\"total\":%d,\"uploaded\":%d,\"total_bytes\":%d,\"uploaded_bytes\":%d,\"status\":\"done\"}\n\n",
				total, uploaded, totalBytes, uploadedBytes)
			flusher.Flush()
			return
		}

		fmt.Fprintf(w, "data: {\"total\":%d,\"uploaded\":%d,\"total_bytes\":%d,\"uploaded_bytes\":%d,\"status\":\"uploading\"}\n\n",
			total, uploaded, totalBytes, uploadedBytes)
		flusher.Flush()
	}
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

	data, filePath, err := h.svc.Download(r.Context(), user.ID, storageID, path)
	if err != nil {
		writeError(w, err)
		return
	}

	filename := filepath.Base(filePath)
	contentType := mime.TypeByExtension(filepath.Ext(filename))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func (h *FilesHandler) Tree(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	storageID, err := parseUUIDParam(r, "storageID")
	if err != nil {
		writeError(w, err)
		return
	}

	path := extractWildcardPath(r)

	elements, err := h.svc.ListDir(r.Context(), user.ID, storageID, path)
	if err != nil {
		writeError(w, err)
		return
	}

	if elements == nil {
		elements = []domain.FSElement{}
	}
	writeJSON(w, http.StatusOK, elements)
}

func (h *FilesHandler) Search(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	storageID, err := parseUUIDParam(r, "storageID")
	if err != nil {
		writeError(w, err)
		return
	}

	basePath := extractWildcardPath(r)
	searchPath := r.URL.Query().Get("search_path")

	results, err := h.svc.Search(r.Context(), user.ID, storageID, basePath, searchPath)
	if err != nil {
		writeError(w, err)
		return
	}

	if results == nil {
		results = []domain.SearchFSElement{}
	}
	writeJSON(w, http.StatusOK, results)
}

func (h *FilesHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
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

	if err := h.svc.Delete(r.Context(), user.ID, storageID, path); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func extractWildcardPath(r *http.Request) string {
	path := chi.URLParam(r, "*")
	path = strings.TrimPrefix(path, "/")
	if decoded, err := url.PathUnescape(path); err == nil {
		path = decoded
	}
	return path
}
