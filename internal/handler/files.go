package handler

import (
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/service"
)

type FilesHandler struct {
	svc *service.FilesService
}

func NewFilesHandler(svc *service.FilesService) *FilesHandler {
	return &FilesHandler{svc: svc}
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

	if err := r.ParseMultipartForm(100 << 20); err != nil { // 100MB max memory
		writeError(w, domain.ErrBadRequest("failed to parse multipart form"))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, domain.ErrBadRequest("file is required"))
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, domain.ErrInternal("failed to read file"))
		return
	}

	path := r.FormValue("path")
	filename := header.Filename

	fullPath := filename
	if path != "" {
		fullPath = path + "/" + filename
	}

	created, err := h.svc.Upload(r.Context(), user.ID, storageID, fullPath, data)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

func (h *FilesHandler) UploadTo(w http.ResponseWriter, r *http.Request) {
	// Same as Upload - handles upload_to endpoint
	h.Upload(w, r)
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

// extractWildcardPath extracts the wildcard portion of the URL path.
func extractWildcardPath(r *http.Request) string {
	path := chi.URLParam(r, "*")
	path = strings.TrimPrefix(path, "/")
	return path
}
