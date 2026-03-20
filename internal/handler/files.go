package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/localfs"
	"github.com/Dominux/Pentaract/internal/pathutil"
	"github.com/Dominux/Pentaract/internal/service"
)

// fileSizeCache caches resolved file sizes to avoid re-downloading the last
// Telegram chunk on every range request (seeking).
type fileSizeCache struct {
	mu    sync.RWMutex
	sizes map[uuid.UUID]int64
}

func newFileSizeCache() *fileSizeCache {
	return &fileSizeCache{sizes: make(map[uuid.UUID]int64)}
}

func (c *fileSizeCache) get(fileID uuid.UUID) (int64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	size, ok := c.sizes[fileID]
	return size, ok
}

func (c *fileSizeCache) set(fileID uuid.UUID, size int64) {
	c.mu.Lock()
	c.sizes[fileID] = size
	c.mu.Unlock()
}

type uploadTracker struct {
	progress  *service.UploadProgress
	cancel    context.CancelFunc
	storageID uuid.UUID
	filePath  string
	err       error
	skipped   bool
	done      bool
}

type downloadTracker struct {
	storageID uuid.UUID
	progress  *service.DownloadProgress
	cancel    context.CancelFunc
	canceled  bool
	err       error
	done      bool
}

type flushWriter struct {
	w       io.Writer
	flusher http.Flusher
}

func (w *flushWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if n > 0 && w.flusher != nil {
		w.flusher.Flush()
	}
	return n, err
}

var inlineVideoContentTypesByExtension = map[string]string{
	".avi":  "video/x-msvideo",
	".flv":  "video/x-flv",
	".m2ts": "video/mp2t",
	".m4v":  "video/x-m4v",
	".mkv":  "video/x-matroska",
	".mov":  "video/quicktime",
	".mp4":  "video/mp4",
	".mpeg": "video/mpeg",
	".mpg":  "video/mpeg",
	".mts":  "video/mp2t",
	".ogg":  "video/ogg",
	".ts":   "video/mp2t",
	".webm": "video/webm",
	".wmv":  "video/x-ms-wmv",
}

type FilesHandler struct {
	svc filesService
	src localFilesSource

	mu        sync.RWMutex
	uploads   map[string]*uploadTracker
	downloads map[string]*downloadTracker

	fileSizes *fileSizeCache
}

type filesService interface {
	EnsureWriteAccess(ctx context.Context, userID, storageID uuid.UUID) error
	Move(ctx context.Context, userID, storageID uuid.UUID, oldPath, newPath string) error
	CreateFolder(ctx context.Context, userID, storageID uuid.UUID, path, folderName string) error
	Upload(ctx context.Context, userID, storageID uuid.UUID, path string, size int64, reader io.Reader, progress *service.UploadProgress, onConflict string) (*domain.File, bool, error)
	Delete(ctx context.Context, userID, storageID uuid.UUID, path string, progress *service.DeleteProgress, forceDelete bool) error
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

type localFilesSource interface {
	ListDir(path string) ([]domain.FSElement, error)
	ExpandSelection(paths []string) ([]domain.FSElement, error)
	OpenFile(path string) (localfs.File, error)
}

func NewFilesHandler(svc *service.FilesService) *FilesHandler {
	return NewFilesHandlerWithService(svc)
}

func NewFilesHandlerWithLocalRoot(svc *service.FilesService, root string) *FilesHandler {
	return NewFilesHandlerWithServiceAndSource(svc, localfs.New(root))
}

func NewFilesHandlerWithService(svc filesService) *FilesHandler {
	return NewFilesHandlerWithServiceAndSource(svc, localfs.New(""))
}

func NewFilesHandlerWithServiceAndSource(svc filesService, src localFilesSource) *FilesHandler {
	return &FilesHandler{
		svc:       svc,
		src:       src,
		uploads:   make(map[string]*uploadTracker),
		downloads: make(map[string]*downloadTracker),
		fileSizes: newFileSizeCache(),
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

type localSelectionRequest struct {
	Paths []string `json:"paths"`
}

type localUploadRequest struct {
	SourcePath string `json:"source_path"`
	TargetPath string `json:"target_path"`
	UploadID   string `json:"upload_id"`
	OnConflict string `json:"on_conflict"`
}

// setupDownloadTracker creates a download tracker from the request's download_id param.
// Returns the context to use, the tracker (may be nil), and a cleanup function to defer.
func (h *FilesHandler) setupDownloadTracker(r *http.Request, storageID uuid.UUID) (context.Context, *downloadTracker, func()) {
	downloadID := r.URL.Query().Get("download_id")
	if downloadID == "" {
		return r.Context(), nil, func() {}
	}
	ctx, cancel := context.WithCancel(r.Context())
	tracker := &downloadTracker{
		storageID: storageID,
		progress:  &service.DownloadProgress{},
		cancel:    cancel,
	}
	h.mu.Lock()
	if previous, ok := h.downloads[downloadID]; ok {
		previous.canceled = true
		previous.done = true
		if previous.cancel != nil {
			previous.cancel()
		}
	}
	h.downloads[downloadID] = tracker
	h.mu.Unlock()
	return ctx, tracker, func() {
		h.scheduleDownloadTrackerCleanup(downloadID, tracker)
	}
}

func (h *FilesHandler) scheduleUploadTrackerCleanup(uploadID string) {
	time.AfterFunc(5*time.Minute, func() {
		h.mu.Lock()
		delete(h.uploads, uploadID)
		h.mu.Unlock()
	})
}

func normalizeUploadRequest(uploadID, onConflict string) (string, string) {
	if uploadID == "" {
		uploadID = uuid.New().String()
	}
	if onConflict == "" {
		onConflict = service.UploadConflictKeepBoth
	}
	return uploadID, onConflict
}

func (h *FilesHandler) setUploadTracker(uploadID string, tracker *uploadTracker) {
	h.mu.Lock()
	h.uploads[uploadID] = tracker
	h.mu.Unlock()
}

func (h *FilesHandler) finishUploadTracker(tracker *uploadTracker, skipped bool, err error) {
	h.mu.Lock()
	tracker.done = true
	tracker.err = err
	tracker.skipped = skipped
	h.mu.Unlock()
}

func writeAcceptedUploadResponse(w http.ResponseWriter, uploadID string) {
	writeJSON(w, http.StatusAccepted, map[string]any{"upload_id": uploadID})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func (h *FilesHandler) startTrackedUpload(userID, storageID uuid.UUID, uploadID, fullPath string, size int64, reader io.Reader, onConflict string, cleanup func(), onError func(error), logMessage string) string {
	uploadID, onConflict = normalizeUploadRequest(uploadID, onConflict)

	uploadCtx, cancel := context.WithCancel(context.Background())
	progress := &service.UploadProgress{TotalBytes: size}
	tracker := &uploadTracker{
		progress:  progress,
		cancel:    cancel,
		storageID: storageID,
		filePath:  fullPath,
	}
	h.setUploadTracker(uploadID, tracker)

	go func() {
		defer h.scheduleUploadTrackerCleanup(uploadID)
		if cleanup != nil {
			defer cleanup()
		}

		_, skipped, uploadErr := h.svc.Upload(uploadCtx, userID, storageID, fullPath, size, reader, progress, onConflict)
		h.finishUploadTracker(tracker, skipped, uploadErr)

		if uploadErr != nil {
			if onError != nil {
				onError(uploadErr)
			}
			log.Printf("[upload] failed %s: %v", logMessage, uploadErr)
		}
	}()

	return uploadID
}

func (h *FilesHandler) cleanupDownloadTracker(downloadID string, tracker *downloadTracker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if current, ok := h.downloads[downloadID]; ok && current == tracker {
		delete(h.downloads, downloadID)
	}
}

func (h *FilesHandler) scheduleDownloadTrackerCleanup(downloadID string, tracker *downloadTracker) {
	time.AfterFunc(5*time.Minute, func() {
		h.cleanupDownloadTracker(downloadID, tracker)
	})
}

// finishTracker marks a download tracker as done with an optional error.
func (h *FilesHandler) finishTracker(tracker *downloadTracker, err error) {
	if tracker == nil {
		return
	}
	h.mu.Lock()
	tracker.done = true
	tracker.err = err
	h.mu.Unlock()
}

func downloadErrorMessage(err error) string {
	if err == nil {
		return ""
	}

	msg := strings.ToLower(err.Error())

	switch {
	case strings.Contains(msg, "exceeds bot api download limit"):
		return "This file was uploaded with an older 20 MB chunk size and Telegram now refuses to download one of its chunks. Re-upload the file to repair it."
	case strings.Contains(msg, "decrypting payload"),
		strings.Contains(msg, "message authentication failed"),
		strings.Contains(msg, "invalid encrypted payload size"):
		return "This file could not be decrypted with the current SECRET_KEY. If the key changed after upload, restore the original key or re-upload the file."
	case strings.Contains(msg, "telegram getfile failed"),
		strings.Contains(msg, "wrong file identifier"),
		strings.Contains(msg, "resolving file_id from message failed"),
		strings.Contains(msg, "forwardmessage failed"),
		strings.Contains(msg, "forwardmessage missing document file_id"):
		return "Telegram could not resolve at least one chunk with the currently available workers. Check that the original bot still exists and still has access to the channel, or re-upload the file."
	case strings.Contains(msg, "reading file data"),
		strings.Contains(msg, "unexpected eof"),
		strings.Contains(msg, "downloading file:"):
		return "Telegram interrupted the download stream for one of the chunks. Please try again."
	default:
		return "Download failed unexpectedly. Please try again."
	}
}

func uploadProgressStatus(progress *service.UploadProgress, done bool, err error, skipped bool) string {
	switch {
	case done && err != nil:
		return "error"
	case done && skipped:
		return "skipped"
	case done:
		return "done"
	case progress != nil && progress.VerificationTotalChunks > 0:
		return "verifying"
	default:
		return "uploading"
	}
}

// setupSSE configures response headers for Server-Sent Events and returns the flusher.
func setupSSE(w http.ResponseWriter) (http.Flusher, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	return flusher, true
}

// writeSSE marshals data as JSON and writes it as an SSE event.
func writeSSE(w http.ResponseWriter, flusher http.Flusher, data any) {
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", b)
	flusher.Flush()
}

// sanitizeFilename returns a safe Content-Disposition filename value.
func sanitizeFilename(name string) string {
	return strings.NewReplacer(`"`, `'`, "\n", "", "\r", "").Replace(name)
}

func (h *FilesHandler) resolvedInlineVideoSize(ctx context.Context, file *domain.File) (int64, error) {
	if totalSize, ok := h.fileSizes.get(file.ID); ok {
		return totalSize, nil
	}
	if file.Size > 0 {
		h.fileSizes.set(file.ID, file.Size)
		return file.Size, nil
	}

	totalSize, err := h.svc.ExactFileSize(ctx, file)
	if err != nil {
		return 0, err
	}

	h.fileSizes.set(file.ID, totalSize)
	return totalSize, nil
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
	fullPath := pathutil.Join(path, filename)

	pr, pw := io.Pipe()
	copyDone := make(chan struct{})

	go func() {
		defer close(copyDone)
		_, err := io.Copy(pw, filePart)
		filePart.Close()
		pw.CloseWithError(err)
	}()

	uploadID = h.startTrackedUpload(
		user.ID,
		storageID,
		uploadID,
		fullPath,
		fileSize,
		pr,
		onConflict,
		nil,
		func(error) { _ = pr.Close() },
		fmt.Sprintf("file=%s", fullPath),
	)
	writeAcceptedUploadResponse(w, uploadID)

	// Keep the handler alive until the body is fully read into the pipe.
	<-copyDone
}

func (h *FilesHandler) LocalTree(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	storageID, err := parseUUIDParam(r, "storageID")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.svc.EnsureWriteAccess(r.Context(), user.ID, storageID); err != nil {
		writeError(w, err)
		return
	}

	path := extractWildcardPath(r)
	elements, err := h.src.ListDir(path)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, nonNilSlice(elements))
}

func (h *FilesHandler) LocalExpandSelection(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	storageID, err := parseUUIDParam(r, "storageID")
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.svc.EnsureWriteAccess(r.Context(), user.ID, storageID); err != nil {
		writeError(w, err)
		return
	}

	var req localSelectionRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if len(req.Paths) == 0 {
		writeError(w, domain.ErrBadRequest("paths are required"))
		return
	}

	files, err := h.src.ExpandSelection(req.Paths)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, nonNilSlice(files))
}

func (h *FilesHandler) LocalUpload(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	storageID, err := parseUUIDParam(r, "storageID")
	if err != nil {
		writeError(w, err)
		return
	}

	var req localUploadRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.SourcePath == "" {
		writeError(w, domain.ErrBadRequest("source_path is required"))
		return
	}

	sourceFile, err := h.src.OpenFile(req.SourcePath)
	if err != nil {
		writeError(w, err)
		return
	}

	targetPath := pathutil.TrimTrailingSlash(req.TargetPath)
	fullPath := pathutil.Join(targetPath, sourceFile.Name)

	pr, pw := io.Pipe()
	copyDone := make(chan struct{})

	go func() {
		defer close(copyDone)
		_, err := io.Copy(pw, sourceFile.Reader)
		sourceFile.Reader.Close()
		pw.CloseWithError(err)
	}()

	uploadID := h.startTrackedUpload(
		user.ID,
		storageID,
		req.UploadID,
		fullPath,
		sourceFile.Size,
		pr,
		req.OnConflict,
		nil,
		func(error) { _ = pr.Close() },
		fmt.Sprintf("local file=%s source=%s", fullPath, sourceFile.Path),
	)
	writeAcceptedUploadResponse(w, uploadID)

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

	log.Printf("[cancel] cancelling upload %s file=%s", uploadID, tracker.filePath)

	go func() {
		time.Sleep(1 * time.Second)
		if err := h.svc.Delete(context.Background(), user.ID, tracker.storageID, tracker.filePath, nil, false); err != nil {
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

	flusher, ok := setupSSE(w)
	if !ok {
		writeError(w, domain.ErrInternal("streaming not supported"))
		return
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}

		h.mu.RLock()
		tracker, exists := h.uploads[uploadID]
		h.mu.RUnlock()

		if !exists {
			writeSSE(w, flusher, map[string]any{"status": "done"})
			return
		}

		p := tracker.progress
		h.mu.RLock()
		isDone := tracker.done
		uploadErr := tracker.err
		isSkipped := tracker.skipped
		h.mu.RUnlock()
		status := uploadProgressStatus(p, isDone, uploadErr, isSkipped)

		writeSSE(w, flusher, map[string]any{
			"total":              p.TotalChunks,
			"uploaded":           p.UploadedChunks.Load(),
			"total_bytes":        p.TotalBytes,
			"uploaded_bytes":     p.UploadedBytes.Load(),
			"verification_total": p.VerificationTotalChunks,
			"verified":           p.VerifiedChunks.Load(),
			"status":             status,
			"workers_status":     h.svc.WorkersStatus(tracker.storageID),
		})

		if isDone {
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

	file, err := h.svc.GetFileForDownload(r.Context(), user.ID, storageID, path)
	if err != nil {
		writeError(w, err)
		return
	}

	downloadCtx, tracker, cleanup := h.setupDownloadTracker(r, storageID)
	defer cleanup()

	filename := filepath.Base(file.Path)
	contentType := contentTypeForFilename(filename)
	disposition := "attachment"
	if r.URL.Query().Get("inline") == "1" {
		disposition = "inline"
	}

	var progress *service.DownloadProgress
	if tracker != nil {
		progress = tracker.progress
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", disposition+`; filename="`+sanitizeFilename(filename)+`"`)

	switch {
	case disposition == "inline" && isInlineVideo(contentType, filename):
		// Video: handle Range requests for streaming.
		w.Header().Set("Accept-Ranges", "bytes")
		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" {
			totalSize, sizeErr := h.resolvedInlineVideoSize(downloadCtx, file)
			if sizeErr != nil {
				log.Printf("[video-size] error: %v", sizeErr)
				writeError(w, domain.ErrInternal("failed to determine exact video size"))
				return
			}
			start, end, err := parseSingleByteRange(rangeHeader, totalSize)
			if err != nil {
				w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", totalSize))
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, totalSize))
			w.Header().Set("Content-Length", fmt.Sprintf("%d", end-start+1))
			w.WriteHeader(http.StatusPartialContent)
			if err := h.svc.DownloadFileRangeToWriter(downloadCtx, file, w, start, end, totalSize, progress); err != nil {
				log.Printf("[download-file-range] error: %v", err)
				h.finishTracker(tracker, err)
				return
			}
		} else {
			if file.Size > 0 {
				w.Header().Set("Content-Length", fmt.Sprintf("%d", file.Size))
			}
			w.WriteHeader(http.StatusOK)
			if err := h.svc.StreamFileToWriter(downloadCtx, file, w, progress); err != nil {
				log.Printf("[stream-file] error: %v", err)
				h.finishTracker(tracker, err)
				return
			}
		}

	case disposition == "inline":
		// Non-video inline: download to temp file, serve with http.ServeContent.
		tmp, err := os.CreateTemp("", "pentaract-file-*")
		if err != nil {
			writeError(w, domain.ErrInternal("failed to create temporary file"))
			return
		}
		tmpPath := tmp.Name()
		defer func() {
			tmp.Close()
			_ = os.Remove(tmpPath)
		}()

		if err := h.svc.DownloadFileToWriter(downloadCtx, file, tmp, progress); err != nil {
			log.Printf("[download-file] error: %v", err)
			h.finishTracker(tracker, err)
			writeError(w, err)
			return
		}

		info, err := tmp.Stat()
		if err != nil {
			writeError(w, domain.ErrInternal("failed to stat temporary file"))
			return
		}
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			writeError(w, domain.ErrInternal("failed to rewind temporary file"))
			return
		}
		w.Header().Set("Accept-Ranges", "bytes")
		http.ServeContent(w, r, filename, info.ModTime(), tmp)

	default:
		// Attachment: stream directly.
		w.WriteHeader(http.StatusOK)
		if err := h.svc.DownloadFileToWriter(downloadCtx, file, w, progress); err != nil {
			log.Printf("[download-file] error: %v", err)
			h.finishTracker(tracker, err)
			return
		}
	}

	h.finishTracker(tracker, nil)
}

func parseSingleByteRange(header string, size int64) (int64, int64, error) {
	if !strings.HasPrefix(header, "bytes=") {
		return 0, 0, fmt.Errorf("invalid range unit")
	}
	rangeSpec := strings.TrimSpace(strings.TrimPrefix(header, "bytes="))
	if rangeSpec == "" || strings.Contains(rangeSpec, ",") {
		return 0, 0, fmt.Errorf("multiple/empty ranges not supported")
	}
	parts := strings.SplitN(rangeSpec, "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range format")
	}

	// Suffix range: bytes=-N
	if parts[0] == "" {
		suffix, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, fmt.Errorf("invalid suffix range")
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, size - 1, nil
	}

	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, fmt.Errorf("invalid range start")
	}

	// Open range: bytes=N-
	if parts[1] == "" {
		return start, size - 1, nil
	}

	end, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || end < start {
		return 0, 0, fmt.Errorf("invalid range end")
	}
	if end >= size {
		end = size - 1
	}
	return start, end, nil
}

func isInlineVideo(contentType, filename string) bool {
	if strings.HasPrefix(strings.ToLower(contentType), "video/") {
		return true
	}
	_, ok := inlineVideoContentTypesByExtension[strings.ToLower(filepath.Ext(filename))]
	return ok
}

func contentTypeForFilename(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if contentType := mime.TypeByExtension(ext); contentType != "" {
		return contentType
	}
	if contentType, ok := inlineVideoContentTypesByExtension[ext]; ok {
		return contentType
	}
	return "application/octet-stream"
}

func (h *FilesHandler) DownloadDir(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	storageID, err := parseUUIDParam(r, "storageID")
	if err != nil {
		writeError(w, err)
		return
	}

	path := extractWildcardPath(r)

	dirName := pathutil.ArchiveName(path)

	downloadCtx, tracker, cleanup := h.setupDownloadTracker(r, storageID)
	defer cleanup()

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, sanitizeFilename(dirName)))

	var progress *service.DownloadProgress
	if tracker != nil {
		progress = tracker.progress
	}

	writer := io.Writer(w)
	if flusher, ok := w.(http.Flusher); ok {
		writer = &flushWriter{w: w, flusher: flusher}
		flusher.Flush()
	}

	if _, err := h.svc.DownloadDir(downloadCtx, user.ID, storageID, path, writer, progress); err != nil {
		log.Printf("[download-dir] error: %v", err)
		h.finishTracker(tracker, err)
		return
	}

	h.finishTracker(tracker, nil)
}

func (h *FilesHandler) CancelDownload(w http.ResponseWriter, r *http.Request) {
	downloadID := chi.URLParam(r, "downloadID")

	h.mu.Lock()
	tracker, exists := h.downloads[downloadID]
	if !exists {
		h.mu.Unlock()
		writeError(w, domain.ErrNotFound("download"))
		return
	}
	tracker.canceled = true
	tracker.done = true
	if tracker.cancel != nil {
		tracker.cancel()
	}
	h.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

// DownloadProgress returns an SSE stream with directory download progress updates.
func (h *FilesHandler) DownloadProgress(w http.ResponseWriter, r *http.Request) {
	downloadID := r.URL.Query().Get("download_id")
	if downloadID == "" {
		writeError(w, domain.ErrBadRequest("download_id is required"))
		return
	}

	flusher, ok := setupSSE(w)
	if !ok {
		writeError(w, domain.ErrInternal("streaming not supported"))
		return
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	waitStart := time.Now()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}

		h.mu.RLock()
		tracker, exists := h.downloads[downloadID]
		h.mu.RUnlock()

		if !exists {
			if time.Since(waitStart) < 15*time.Second {
				writeSSE(w, flusher, map[string]any{
					"total": 0, "downloaded": 0, "total_bytes": 0, "downloaded_bytes": 0, "status": "downloading",
				})
				continue
			}
			writeSSE(w, flusher, map[string]any{
				"status":        "error",
				"error_message": "Download did not start on the server. Please try again.",
			})
			return
		}

		p := tracker.progress
		status := "downloading"

		h.mu.RLock()
		isDone := tracker.done
		downloadErr := tracker.err
		isCanceled := tracker.canceled
		h.mu.RUnlock()

		if isDone && isCanceled {
			status = "cancelled"
		} else if isDone && downloadErr != nil {
			status = "error"
		} else if isDone {
			status = "done"
		}

		writeSSE(w, flusher, map[string]any{
			"total":            p.TotalChunks,
			"downloaded":       p.DownloadedChunks.Load(),
			"total_bytes":      p.TotalBytes,
			"downloaded_bytes": p.DownloadedBytes.Load(),
			"status":           status,
			"workers_status":   h.svc.WorkersStatus(tracker.storageID),
			"error_message":    downloadErrorMessage(downloadErr),
		})

		if isDone {
			return
		}
	}
}

// DeleteProgress returns an SSE stream with delete progress updates.
func (h *FilesHandler) DeleteProgress(w http.ResponseWriter, r *http.Request) {
	deleteID := r.URL.Query().Get("delete_id")
	if deleteID == "" {
		writeError(w, domain.ErrBadRequest("delete_id is required"))
		return
	}

	flusher, ok := setupSSE(w)
	if !ok {
		writeError(w, domain.ErrInternal("streaming not supported"))
		return
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	waitStart := time.Now()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}

		tracker, exists := getDeleteTracker(deleteID)
		if !exists {
			if time.Since(waitStart) < 15*time.Second {
				writeSSE(w, flusher, map[string]any{
					"total": 0, "deleted": 0, "pending": 0, "status": "deleting",
				})
				continue
			}
			writeSSE(w, flusher, map[string]any{"status": "error"})
			return
		}

		done, trackerErr, total, deleted := getDeleteTrackerStatus(tracker)
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

		writeSSE(w, flusher, map[string]any{
			"total":          total,
			"deleted":        deleted,
			"pending":        pending,
			"status":         status,
			"workers_status": h.svc.WorkersStatus(tracker.storageID),
		})

		if done {
			return
		}
	}
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
