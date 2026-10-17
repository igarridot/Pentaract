package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	appjwt "github.com/Dominux/Pentaract/internal/jwt"
	"github.com/Dominux/Pentaract/internal/pathutil"
)

// maxLocalUploadBatch bounds how many files one batch request may queue.
const maxLocalUploadBatch = 100

// safePath resolves requestedPath relative to basePath and ensures it does not
// escape basePath (preventing path-traversal attacks).
func safePath(basePath, requestedPath string) (string, error) {
	if basePath == "" {
		return "", fmt.Errorf("base path is not configured")
	}

	abs, err := filepath.Abs(filepath.Join(basePath, requestedPath))
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("failed to evaluate symlinks: %w", err)
	}

	// Ensure the resolved path is within the base path.
	if !strings.HasPrefix(resolved, basePath) {
		return "", fmt.Errorf("path escapes base directory")
	}

	return resolved, nil
}

// resolveLocalFile validates a local upload source: it must stay inside the
// mount and be a regular file.
func resolveLocalFile(basePath, localPath string) (string, os.FileInfo, error) {
	if localPath == "" {
		return "", nil, domain.ErrBadRequest("local_path is required")
	}
	resolved, err := safePath(basePath, localPath)
	if err != nil {
		return "", nil, domain.ErrBadRequest(fmt.Sprintf("invalid local_path %q: %s", localPath, err.Error()))
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", nil, domain.ErrBadRequest(fmt.Sprintf("cannot stat %q: %s", localPath, err.Error()))
	}
	if info.IsDir() {
		return "", nil, domain.ErrBadRequest(fmt.Sprintf("%q is a directory, not a file", localPath))
	}
	return resolved, info, nil
}

// BrowseLocalFS lists files and directories at the given path on the local
// filesystem (relative to the configured base path).
func (h *UploadHandler) BrowseLocalFS(w http.ResponseWriter, r *http.Request) {
	if h.localBasePath == "" {
		writeError(w, domain.ErrForbidden())
		return
	}

	resolvedPath, err := safePath(h.localBasePath, r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, domain.ErrBadRequest(err.Error()))
		return
	}

	entries, err := os.ReadDir(resolvedPath)
	if err != nil {
		writeError(w, domain.ErrBadRequest("cannot read directory: "+err.Error()))
		return
	}

	elements := make([]domain.FSElement, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			// Skip entries that fail to stat (permission errors, etc.)
			continue
		}

		relPath, err := filepath.Rel(h.localBasePath, filepath.Join(resolvedPath, entry.Name()))
		if err != nil {
			continue
		}

		elem := domain.FSElement{
			Name:   entry.Name(),
			Path:   relPath,
			IsFile: !entry.IsDir(),
		}
		if elem.IsFile {
			elem.Size = info.Size()
		}
		elements = append(elements, elem)
	}

	writeJSON(w, http.StatusOK, elements)
}

// localUploadRequest is the preamble shared by the local upload endpoints:
// the mount check, then the storage route and the JSON body.
func (h *UploadHandler) localUploadRequest(r *http.Request, dst any) (*appjwt.AuthUser, uuid.UUID, error) {
	if h.localBasePath == "" {
		return nil, uuid.Nil, domain.ErrForbidden()
	}
	return storageJSONRequest(r, dst)
}

// localUpload is one validated file from the local mount, ready to queue.
type localUpload struct {
	uploadID     string
	resolvedPath string
	fullPath     string
	size         int64
}

// resolveLocalUpload validates one local upload and derives its destination.
func (h *UploadHandler) resolveLocalUpload(localPath, destPath, uploadID string) (localUpload, error) {
	resolvedPath, info, err := resolveLocalFile(h.localBasePath, localPath)
	if err != nil {
		return localUpload{}, err
	}
	return localUpload{
		uploadID:     uploadIDOrNew(uploadID),
		resolvedPath: resolvedPath,
		fullPath:     pathutil.Join(pathutil.TrimTrailingSlash(destPath), info.Name()),
		size:         info.Size(),
	}, nil
}

// queueLocalUploads registers a tracker for every item, so each upload_id can
// be subscribed to (and cancelled) right away, then uploads the items one at
// a time in the background, like browser uploads: parallel files would
// compete for Telegram API bandwidth and time out.
func (h *UploadHandler) queueLocalUploads(userID, storageID uuid.UUID, items []localUpload, onConflict string) {
	type queuedUpload struct {
		item    localUpload
		tracker *uploadTracker
		ctx     context.Context
	}
	queue := make([]queuedUpload, 0, len(items))
	for _, item := range items {
		tracker, uploadCtx := h.registerUpload(item.uploadID, storageID, item.fullPath, item.size)
		queue = append(queue, queuedUpload{item: item, tracker: tracker, ctx: uploadCtx})
	}

	go func() {
		for _, q := range queue {
			if err := q.ctx.Err(); err != nil {
				// Cancelled while still queued: report it without starting.
				h.finishUpload(q.tracker, nil, false, err)
			} else {
				h.uploadLocalFile(q.ctx, q.tracker, userID, storageID, q.item, onConflict)
			}
			h.scheduleUploadTrackerCleanup(q.item.uploadID)
		}
	}()
}

// uploadLocalFile opens a file from the local mount and runs a tracked upload.
func (h *UploadHandler) uploadLocalFile(ctx context.Context, tracker *uploadTracker, userID, storageID uuid.UUID, item localUpload, onConflict string) {
	f, err := os.Open(item.resolvedPath)
	if err != nil {
		slog.Error("local upload: failed to open file", "file", item.resolvedPath, "err", err)
		h.finishUpload(tracker, nil, false, err)
		return
	}
	h.runTrackedUpload(ctx, tracker, userID, storageID, item.fullPath, item.size, f, onConflict)
}

type uploadLocalRequest struct {
	LocalPath  string `json:"local_path"`
	DestPath   string `json:"dest_path"`
	UploadID   string `json:"upload_id"`
	OnConflict string `json:"on_conflict"`
}

// UploadLocal uploads a single file from the container's local filesystem to
// Telegram storage. It returns immediately with a 202 and an upload_id that
// can be used to track progress via /api/upload_progress.
func (h *UploadHandler) UploadLocal(w http.ResponseWriter, r *http.Request) {
	var req uploadLocalRequest
	user, storageID, err := h.localUploadRequest(r, &req)
	if err != nil {
		writeError(w, err)
		return
	}

	item, err := h.resolveLocalUpload(req.LocalPath, req.DestPath, req.UploadID)
	if err != nil {
		writeError(w, err)
		return
	}

	h.queueLocalUploads(user.ID, storageID, []localUpload{item}, conflictPolicy(req.OnConflict))
	writeJSON(w, http.StatusAccepted, map[string]any{"upload_id": item.uploadID})
}

type uploadLocalBatchItem struct {
	LocalPath string `json:"local_path"`
	DestPath  string `json:"dest_path"`
}

type uploadLocalBatchRequest struct {
	Items      []uploadLocalBatchItem `json:"items"`
	OnConflict string                 `json:"on_conflict"`
}

// UploadLocalBatch starts multiple local file uploads at once.
func (h *UploadHandler) UploadLocalBatch(w http.ResponseWriter, r *http.Request) {
	var req uploadLocalBatchRequest
	user, storageID, err := h.localUploadRequest(r, &req)
	if err != nil {
		writeError(w, err)
		return
	}

	if len(req.Items) == 0 {
		writeError(w, domain.ErrBadRequest("items is required"))
		return
	}
	if len(req.Items) > maxLocalUploadBatch {
		writeError(w, domain.ErrBadRequest(fmt.Sprintf("batch size must not exceed %d", maxLocalUploadBatch)))
		return
	}

	type uploadResult struct {
		LocalPath string `json:"local_path"`
		UploadID  string `json:"upload_id"`
	}

	// Validate every path before queueing anything (fail fast).
	items := make([]localUpload, 0, len(req.Items))
	results := make([]uploadResult, 0, len(req.Items))
	for _, reqItem := range req.Items {
		item, err := h.resolveLocalUpload(reqItem.LocalPath, reqItem.DestPath, "")
		if err != nil {
			writeError(w, err)
			return
		}
		items = append(items, item)
		results = append(results, uploadResult{LocalPath: reqItem.LocalPath, UploadID: item.uploadID})
	}

	h.queueLocalUploads(user.ID, storageID, items, conflictPolicy(req.OnConflict))
	writeJSON(w, http.StatusAccepted, map[string]any{"uploads": results})
}
