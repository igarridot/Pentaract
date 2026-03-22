package handler

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/pathutil"
	"github.com/Dominux/Pentaract/internal/service"
)

type FilesHandler struct {
	svc filesService

	mu        sync.RWMutex
	uploads   map[string]*uploadTracker
	downloads map[string]*downloadTracker

	fileSizes     *fileSizeCache
	localBasePath string
}

type filesService interface {
	Move(ctx context.Context, userID, storageID uuid.UUID, oldPath, newPath string) error
	CreateFolder(ctx context.Context, userID, storageID uuid.UUID, path, folderName string) error
	Upload(ctx context.Context, userID, storageID uuid.UUID, path string, size int64, reader io.Reader, progress *service.UploadProgress, onConflict string) (*domain.File, bool, error)
	Delete(ctx context.Context, userID, storageID uuid.UUID, path string, progress *service.DeleteProgress, forceDelete bool) error
	CleanupCancelledUpload(ctx context.Context, userID, storageID uuid.UUID, path string) error
	WorkersStatus(storageID uuid.UUID) string
	GetFileForDownload(ctx context.Context, userID, storageID uuid.UUID, path string) (*domain.File, error)
	ExactFileSize(ctx context.Context, file *domain.File) (int64, error)
	DownloadFileRangeToWriter(ctx context.Context, file *domain.File, w io.Writer, start, end, totalSize int64, progress *service.DownloadProgress) error
	DownloadFileToWriter(ctx context.Context, file *domain.File, w io.Writer, progress *service.DownloadProgress) error
	StreamFileToWriter(ctx context.Context, file *domain.File, w io.Writer, progress *service.DownloadProgress) error
	DownloadDir(ctx context.Context, userID, storageID uuid.UUID, dirPath string, w io.Writer, progress *service.DownloadProgress) (string, error)
	ListDir(ctx context.Context, userID, storageID uuid.UUID, path string) ([]domain.FSElement, error)
	Search(ctx context.Context, userID, storageID uuid.UUID, basePath, searchPath string) ([]domain.FSElement, error)
}

// LocalUploadMountPath is the fixed mount point inside the container where
// the host directory specified by LOCAL_UPLOAD_BASE_PATH is mounted.
const LocalUploadMountPath = "/mnt/data"

func NewFilesHandler(svc filesService) *FilesHandler {
	// Auto-detect local upload support: enabled when the mount point exists.
	basePath := ""
	if info, err := os.Stat(LocalUploadMountPath); err == nil && info.IsDir() {
		basePath = LocalUploadMountPath
	}

	return &FilesHandler{
		svc:           svc,
		uploads:       make(map[string]*uploadTracker),
		downloads:     make(map[string]*downloadTracker),
		fileSizes:     newFileSizeCache(),
		localBasePath: basePath,
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
	user := GetAuthUser(r.Context())
	storageID, err := parseUUIDParam(r, "storageID")
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

	writeJSON(w, http.StatusOK, nonNilSlice(elements))
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

	writeJSON(w, http.StatusOK, nonNilSlice(results))
}

func (h *FilesHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	storageID, err := parseUUIDParam(r, "storageID")
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

	deleteID := r.URL.Query().Get("delete_id")
	forceDelete, err := strconv.ParseBool(r.URL.Query().Get("force_delete"))
	if err != nil && r.URL.Query().Get("force_delete") != "" {
		writeError(w, domain.ErrBadRequest("invalid force_delete value"))
		return
	}
	var tracker *deleteTracker
	if deleteID != "" {
		tracker = startDeleteTracker(deleteID, storageID)
		defer scheduleDeleteTrackerCleanup(deleteID)
	}

	var progress *service.DeleteProgress
	if tracker != nil {
		progress = tracker.progress
	}

	if err := h.svc.Delete(r.Context(), user.ID, storageID, path, progress, forceDelete); err != nil {
		if tracker != nil {
			markDeleteTrackerDone(tracker, err)
		}
		writeError(w, err)
		return
	}

	if tracker != nil {
		markDeleteTrackerDone(tracker, nil)
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
