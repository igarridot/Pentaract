package handler

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/pathutil"
	"github.com/Dominux/Pentaract/internal/service"
)

// FilesHandler serves the file browser API. Uploads and downloads live in
// their own handlers; this type embeds them so routing and tests see one
// object, and adds the metadata operations (tree, search, move, delete).
type FilesHandler struct {
	*UploadHandler
	*DownloadHandler

	svc     filesService
	deletes *DeleteTrackers
}

// Service slices per handler, so each one only sees what it uses.
type (
	uploadService interface {
		Upload(ctx context.Context, userID, storageID uuid.UUID, path string, size int64, reader io.Reader, progress *service.UploadProgress, onConflict string) (*domain.File, bool, error)
		CleanupCancelledUpload(ctx context.Context, userID, storageID uuid.UUID, fileID uuid.UUID) error
		WorkersStatus(storageID uuid.UUID) string
	}
	downloadService interface {
		GetFileForDownload(ctx context.Context, userID, storageID uuid.UUID, path string) (*domain.File, error)
		ExactFileSize(ctx context.Context, file *domain.File) (int64, error)
		DownloadFileRangeToWriter(ctx context.Context, file *domain.File, w io.Writer, start, end, totalSize int64, progress *service.DownloadProgress) error
		DownloadFileToWriter(ctx context.Context, file *domain.File, w io.Writer, progress *service.DownloadProgress) error
		StreamFileToWriter(ctx context.Context, file *domain.File, w io.Writer, progress *service.DownloadProgress) error
		DownloadDir(ctx context.Context, userID, storageID uuid.UUID, dirPath string, w io.Writer, progress *service.DownloadProgress) (string, error)
		WorkersStatus(storageID uuid.UUID) string
	}
	filesService interface {
		uploadService
		downloadService
		Move(ctx context.Context, userID, storageID uuid.UUID, oldPath, newPath string) error
		CreateFolder(ctx context.Context, userID, storageID uuid.UUID, path, folderName string) error
		Delete(ctx context.Context, userID, storageID uuid.UUID, path string, progress *service.DeleteProgress, forceDelete bool) error
		ListDir(ctx context.Context, userID, storageID uuid.UUID, path string) ([]domain.FSElement, error)
		Search(ctx context.Context, userID, storageID uuid.UUID, basePath, searchPath string) ([]domain.FSElement, error)
	}
)

// NewFilesHandler builds the files handler. deletes is shared with the
// storages handler so both report through the same delete progress stream.
func NewFilesHandler(svc filesService, deletes *DeleteTrackers) *FilesHandler {
	return &FilesHandler{
		UploadHandler:   NewUploadHandler(svc),
		DownloadHandler: NewDownloadHandler(svc),
		svc:             svc,
		deletes:         deletes,
	}
}

type createFolderRequest struct {
	Path       string `json:"path"`
	FolderName string `json:"folder_name"`
}

type moveFileRequest struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
}

func (h *FilesHandler) Move(w http.ResponseWriter, r *http.Request) {
	user, storageID, err := storageRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	var req moveFileRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, err)
		return
	}

	if req.OldPath == "" || req.NewPath == "" {
		writeError(w, domain.ErrBadRequest("old_path and new_path are required"))
		return
	}

	if err := h.svc.Move(r.Context(), user.ID, storageID, req.OldPath, req.NewPath); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *FilesHandler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	user, storageID, err := storageRequest(r)
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

func (h *FilesHandler) Tree(w http.ResponseWriter, r *http.Request) {
	user, storageID, err := storageRequest(r)
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

	writeJSON(w, http.StatusOK, nonNilSlice(elements))
}

func (h *FilesHandler) Search(w http.ResponseWriter, r *http.Request) {
	user, storageID, err := storageRequest(r)
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

	writeJSON(w, http.StatusOK, nonNilSlice(results))
}

func (h *FilesHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	user, storageID, err := storageRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	path := extractWildcardPath(r)
	path = pathutil.TrimTrailingSlash(path)
	if path == "" {
		writeError(w, domain.ErrBadRequest("file path is required"))
		return
	}

	forceDelete, err := strconv.ParseBool(r.URL.Query().Get("force_delete"))
	if err != nil && r.URL.Query().Get("force_delete") != "" {
		writeError(w, domain.ErrBadRequest("invalid force_delete value"))
		return
	}

	progress, finish := h.deletes.track(r.URL.Query().Get("delete_id"), storageID)
	err = h.svc.Delete(r.Context(), user.ID, storageID, path, progress, forceDelete)
	finish(err)
	if err != nil {
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

// DeleteProgress returns an SSE stream with delete progress updates.
func (h *FilesHandler) DeleteProgress(w http.ResponseWriter, r *http.Request) {
	deleteID := r.URL.Query().Get("delete_id")

	pollSSE(w, r, "delete_id",
		map[string]any{
			"total": 0, "deleted": 0, "pending": 0, "status": "deleting",
		},
		map[string]any{"status": "error"},
		func() (map[string]any, bool, bool) {
			tracker, exists := h.deletes.get(deleteID)
			if !exists {
				return nil, false, false
			}

			done, trackerErr, total, deleted := tracker.status()
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
