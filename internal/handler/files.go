package handler

import (
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/service"
)

type FilesHandler struct {
	svc *service.FilesService

	// In-flight upload progress tracking
	mu       sync.RWMutex
	progress map[string]*service.UploadProgress // keyed by upload ID
}

func NewFilesHandler(svc *service.FilesService) *FilesHandler {
	return &FilesHandler{
		svc:      svc,
		progress: make(map[string]*service.UploadProgress),
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

	// Parse multipart: files larger than 32MB are spilled to disk by Go automatically
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, domain.ErrBadRequest("multipart form required"))
		return
	}

	var path, filename string
	var fileSize int64
	var fileReader *partReader

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
		case "file":
			filename = part.FileName()
			fileReader = &partReader{part: part}
			// Content-Length is not available per-part; use the header as hint
			fileSize = r.ContentLength
			if fileSize < 0 {
				fileSize = 0
			}
		}
		if fileReader != nil && filename != "" {
			break // we have what we need, start streaming
		}
	}

	if fileReader == nil || filename == "" {
		writeError(w, domain.ErrBadRequest("file is required"))
		return
	}
	defer fileReader.part.Close()

	fullPath := filename
	if path != "" {
		fullPath = path + "/" + filename
	}

	// Create progress tracker
	uploadID := uuid.New().String()
	progress := &service.UploadProgress{}
	h.mu.Lock()
	h.progress[uploadID] = progress
	h.mu.Unlock()
	defer func() {
		// Keep progress around briefly so the SSE client can read the final state
		time.AfterFunc(30*time.Second, func() {
			h.mu.Lock()
			delete(h.progress, uploadID)
			h.mu.Unlock()
		})
	}()

	created, err := h.svc.Upload(r.Context(), user.ID, storageID, fullPath, fileSize, fileReader, progress)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":        created.ID,
		"path":      created.Path,
		"size":      created.Size,
		"upload_id": uploadID,
	})
}

func (h *FilesHandler) UploadTo(w http.ResponseWriter, r *http.Request) {
	h.Upload(w, r)
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
		p, exists := h.progress[uploadID]
		h.mu.RUnlock()

		if !exists {
			fmt.Fprintf(w, "data: {\"status\":\"done\"}\n\n")
			flusher.Flush()
			return
		}

		total := p.TotalChunks
		uploaded := p.UploadedChunks.Load()

		fmt.Fprintf(w, "data: {\"total\":%d,\"uploaded\":%d,\"status\":\"uploading\"}\n\n", total, uploaded)
		flusher.Flush()

		if total > 0 && uploaded >= total {
			fmt.Fprintf(w, "data: {\"total\":%d,\"uploaded\":%d,\"status\":\"done\"}\n\n", total, uploaded)
			flusher.Flush()
			return
		}
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
	return path
}

// partReader wraps a multipart.Part as an io.Reader.
type partReader struct {
	part interface {
		Read([]byte) (int, error)
		Close() error
	}
}

func (pr *partReader) Read(p []byte) (int, error) {
	return pr.part.Read(p)
}
