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
	"github.com/Dominux/Pentaract/internal/pathutil"
	"github.com/Dominux/Pentaract/internal/service"
)

// safePath resolves requestedPath relative to basePath and ensures it does not
// escape basePath (preventing path-traversal attacks).
func safePath(basePath, requestedPath string) (string, error) {
	if basePath == "" {
		return "", fmt.Errorf("base path is not configured")
	}

	joined := filepath.Join(basePath, requestedPath)

	abs, err := filepath.Abs(joined)
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
func (h *FilesHandler) BrowseLocalFS(w http.ResponseWriter, r *http.Request) {
	if h.localBasePath == "" {
		writeError(w, domain.ErrForbidden())
		return
	}

	requestedPath := r.URL.Query().Get("path")

	resolvedPath, err := safePath(h.localBasePath, requestedPath)
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

		fullPath := filepath.Join(resolvedPath, entry.Name())
		relPath, err := filepath.Rel(h.localBasePath, fullPath)
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

type uploadLocalRequest struct {
	LocalPath  string `json:"local_path"`
	DestPath   string `json:"dest_path"`
	UploadID   string `json:"upload_id"`
	OnConflict string `json:"on_conflict"`
}

// UploadLocal uploads a single file from the container's local filesystem to
// Telegram storage. It returns immediately with a 202 and an upload_id that
// can be used to track progress via /api/upload_progress.
func (h *FilesHandler) UploadLocal(w http.ResponseWriter, r *http.Request) {
	user, storageID, err := storageRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	if h.localBasePath == "" {
		writeError(w, domain.ErrForbidden())
		return
	}

	var req uploadLocalRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, err)
		return
	}

	resolvedPath, info, err := resolveLocalFile(h.localBasePath, req.LocalPath)
	if err != nil {
		writeError(w, err)
		return
	}

	destPath := pathutil.TrimTrailingSlash(req.DestPath)
	fullPath := pathutil.Join(destPath, info.Name())

	onConflict := req.OnConflict
	if onConflict == "" {
		onConflict = service.UploadConflictKeepBoth
	}

	uploadID := req.UploadID
	if uploadID == "" {
		uploadID = uuid.New().String()
	}

	tracker, uploadCtx := h.registerUpload(uploadID, storageID, fullPath, info.Size())
	go func() {
		defer h.scheduleUploadTrackerCleanup(uploadID)
		h.uploadLocalFile(uploadCtx, tracker, user.ID, storageID, resolvedPath, fullPath, info.Size(), onConflict)
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{"upload_id": uploadID})
}

// uploadLocalFile opens a file from the local mount and runs a tracked upload.
func (h *FilesHandler) uploadLocalFile(ctx context.Context, tracker *uploadTracker, userID, storageID uuid.UUID, localPath, fullPath string, fileSize int64, onConflict string) {
	f, err := os.Open(localPath)
	if err != nil {
		slog.Error("local upload: failed to open file", "file", localPath, "err", err)
		h.finishUpload(tracker, nil, false, err)
		return
	}
	h.runTrackedUpload(ctx, tracker, userID, storageID, fullPath, fileSize, f, onConflict)
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
func (h *FilesHandler) UploadLocalBatch(w http.ResponseWriter, r *http.Request) {
	user, storageID, err := storageRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}

	if h.localBasePath == "" {
		writeError(w, domain.ErrForbidden())
		return
	}

	var req uploadLocalBatchRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, err)
		return
	}

	if len(req.Items) == 0 {
		writeError(w, domain.ErrBadRequest("items is required"))
		return
	}
	if len(req.Items) > 100 {
		writeError(w, domain.ErrBadRequest("batch size must not exceed 100"))
		return
	}

	onConflict := req.OnConflict
	if onConflict == "" {
		onConflict = service.UploadConflictKeepBoth
	}

	// Validate all paths first (fail fast).
	type resolvedItem struct {
		resolvedPath string
		info         os.FileInfo
		fullPath     string
	}
	resolved := make([]resolvedItem, 0, len(req.Items))
	for _, item := range req.Items {
		rp, info, err := resolveLocalFile(h.localBasePath, item.LocalPath)
		if err != nil {
			writeError(w, err)
			return
		}
		resolved = append(resolved, resolvedItem{
			resolvedPath: rp,
			info:         info,
			fullPath:     pathutil.Join(pathutil.TrimTrailingSlash(item.DestPath), info.Name()),
		})
	}

	type uploadResult struct {
		LocalPath string `json:"local_path"`
		UploadID  string `json:"upload_id"`
	}
	type queuedUpload struct {
		uploadID string
		item     resolvedItem
		tracker  *uploadTracker
		ctx      context.Context
	}

	// Pre-register every tracker so each upload_id can be subscribed to (and
	// cancelled) right away, even for items still waiting in the queue.
	results := make([]uploadResult, 0, len(resolved))
	queue := make([]queuedUpload, 0, len(resolved))
	for i, ri := range resolved {
		uploadID := uuid.New().String()
		tracker, uploadCtx := h.registerUpload(uploadID, storageID, ri.fullPath, ri.info.Size())
		queue = append(queue, queuedUpload{uploadID: uploadID, item: ri, tracker: tracker, ctx: uploadCtx})
		results = append(results, uploadResult{LocalPath: req.Items[i].LocalPath, UploadID: uploadID})
	}

	// Process files sequentially in a single goroutine, matching browser upload
	// behavior. This prevents multiple files from competing for Telegram API
	// bandwidth and avoids upload timeouts.
	go func() {
		for _, q := range queue {
			if err := q.ctx.Err(); err != nil {
				// Cancelled while still queued: report it without starting.
				h.finishUpload(q.tracker, nil, false, err)
			} else {
				h.uploadLocalFile(q.ctx, q.tracker, user.ID, storageID, q.item.resolvedPath, q.item.fullPath, q.item.info.Size(), onConflict)
			}
			h.scheduleUploadTrackerCleanup(q.uploadID)
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{"uploads": results})
}
