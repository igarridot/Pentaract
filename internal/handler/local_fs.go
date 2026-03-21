package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	user := GetAuthUser(r.Context())
	storageID, err := parseUUIDParam(r, "storageID")
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

	if req.LocalPath == "" {
		writeError(w, domain.ErrBadRequest("local_path is required"))
		return
	}

	resolvedPath, err := safePath(h.localBasePath, req.LocalPath)
	if err != nil {
		writeError(w, domain.ErrBadRequest(err.Error()))
		return
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		writeError(w, domain.ErrBadRequest("cannot stat file: "+err.Error()))
		return
	}
	if info.IsDir() {
		writeError(w, domain.ErrBadRequest("local_path must be a file, not a directory"))
		return
	}

	fileSize := info.Size()
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

	progress := &service.UploadProgress{TotalBytes: fileSize}
	uploadCtx, cancel := context.WithCancel(context.Background())
	tracker := &uploadTracker{
		progress:  progress,
		cancel:    cancel,
		storageID: storageID,
		filePath:  fullPath,
	}

	h.mu.Lock()
	h.uploads[uploadID] = tracker
	h.mu.Unlock()

	// Start a goroutine that opens the file, pipes it, and uploads.
	go func() {
		defer h.scheduleUploadTrackerCleanup(uploadID)

		f, err := os.Open(resolvedPath)
		if err != nil {
			slog.Error("local upload: failed to open file", "file", resolvedPath, "err", err)
			h.mu.Lock()
			tracker.done = true
			tracker.err = err
			h.mu.Unlock()
			return
		}

		pr, pw := io.Pipe()
		// S3: Buffer one chunk ahead so the file reader can race ahead of
		// the chunk encryption/upload pipeline, smoothing throughput.
		go func() {
			bw := bufio.NewWriterSize(pw, service.UploadChunkSize)
			_, copyErr := io.Copy(bw, f)
			if flushErr := bw.Flush(); copyErr == nil {
				copyErr = flushErr
			}
			f.Close()
			pw.CloseWithError(copyErr)
		}()

		_, skipped, uploadErr := h.svc.Upload(uploadCtx, user.ID, storageID, fullPath, fileSize, pr, progress, onConflict)

		h.mu.Lock()
		tracker.done = true
		tracker.err = uploadErr
		tracker.skipped = skipped
		h.mu.Unlock()

		if uploadErr != nil {
			slog.Error("local upload failed", "file", fullPath, "err", uploadErr)
			pr.Close()
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{"upload_id": uploadID})
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
	user := GetAuthUser(r.Context())
	storageID, err := parseUUIDParam(r, "storageID")
	if err != nil {
		writeError(w, err)
		return
	}

	if h.localBasePath == "" {
		writeError(w, domain.ErrForbidden())
		return
	}

	var req uploadLocalBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, domain.ErrBadRequest("invalid request body"))
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
		destPath     string
		fullPath     string
	}
	resolved := make([]resolvedItem, 0, len(req.Items))
	for _, item := range req.Items {
		if item.LocalPath == "" {
			writeError(w, domain.ErrBadRequest("local_path is required for each item"))
			return
		}
		rp, err := safePath(h.localBasePath, item.LocalPath)
		if err != nil {
			writeError(w, domain.ErrBadRequest(fmt.Sprintf("invalid local_path %q: %s", item.LocalPath, err.Error())))
			return
		}
		info, err := os.Stat(rp)
		if err != nil {
			writeError(w, domain.ErrBadRequest(fmt.Sprintf("cannot stat %q: %s", item.LocalPath, err.Error())))
			return
		}
		if info.IsDir() {
			writeError(w, domain.ErrBadRequest(fmt.Sprintf("%q is a directory, not a file", item.LocalPath)))
			return
		}
		dp := pathutil.TrimTrailingSlash(item.DestPath)
		fp := pathutil.Join(dp, info.Name())
		resolved = append(resolved, resolvedItem{
			resolvedPath: rp,
			info:         info,
			destPath:     dp,
			fullPath:     fp,
		})
	}

	type uploadResult struct {
		LocalPath string `json:"local_path"`
		UploadID  string `json:"upload_id"`
	}
	results := make([]uploadResult, 0, len(resolved))

	// Pre-create all trackers so every upload_id is immediately available for
	// SSE progress subscriptions.
	type trackerEntry struct {
		uploadID string
		tracker  *uploadTracker
		progress *service.UploadProgress
		cancel   context.CancelFunc
	}
	trackers := make([]trackerEntry, len(resolved))
	for i, ri := range resolved {
		uploadID := uuid.New().String()
		progress := &service.UploadProgress{TotalBytes: ri.info.Size()}
		uploadCtx, cancel := context.WithCancel(context.Background())
		tracker := &uploadTracker{
			progress:  progress,
			cancel:    cancel,
			storageID: storageID,
			filePath:  ri.fullPath,
		}

		h.mu.Lock()
		h.uploads[uploadID] = tracker
		h.mu.Unlock()

		trackers[i] = trackerEntry{uploadID: uploadID, tracker: tracker, progress: progress, cancel: cancel}
		results = append(results, uploadResult{
			LocalPath: req.Items[i].LocalPath,
			UploadID:  uploadID,
		})
		_ = uploadCtx // used by cancel
	}

	// Process files sequentially in a single goroutine, matching browser upload
	// behavior. This prevents multiple files from competing for Telegram API
	// bandwidth and avoids upload timeouts.
	go func() {
		for i, ri := range resolved {
			te := trackers[i]

			f, err := os.Open(ri.resolvedPath)
			if err != nil {
				slog.Error("local batch upload: failed to open file", "file", ri.resolvedPath, "err", err)
				h.mu.Lock()
				te.tracker.done = true
				te.tracker.err = err
				h.mu.Unlock()
				h.scheduleUploadTrackerCleanup(te.uploadID)
				continue
			}

			pr, pw := io.Pipe()
			go func() {
				_, copyErr := io.Copy(pw, f)
				f.Close()
				pw.CloseWithError(copyErr)
			}()

			uploadCtx, cancel := context.WithCancel(context.Background())
			te.tracker.cancel = cancel

			_, skipped, uploadErr := h.svc.Upload(uploadCtx, user.ID, storageID, ri.fullPath, ri.info.Size(), pr, te.progress, onConflict)

			h.mu.Lock()
			te.tracker.done = true
			te.tracker.err = uploadErr
			te.tracker.skipped = skipped
			h.mu.Unlock()

			if uploadErr != nil {
				slog.Error("local batch upload failed", "file", ri.fullPath, "err", uploadErr)
				pr.Close()
			}

			h.scheduleUploadTrackerCleanup(te.uploadID)
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{"uploads": results})
}
