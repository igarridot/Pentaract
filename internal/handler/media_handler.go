package handler

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/media"
	"github.com/Dominux/Pentaract/internal/service"
)

// hlsPlayer is the slice of service.HLSService the media handler uses.
type hlsPlayer interface {
	Available() (bool, string)
	SegmentSeconds() int
	MediaInfo(ctx context.Context, file *domain.File) (*media.Info, error)
	Playlist(ctx context.Context, file *domain.File, audio uint64, query string) ([]byte, error)
	Resource(ctx context.Context, userID uuid.UUID, file *domain.File, audio uint64, name string) (string, error)
	Stop(userID, fileID uuid.UUID)
	LookupSource(token string) (*domain.File, bool)
}

// MediaHandler describes media files and plays Matroska files through HLS.
// Everything else (MP4, images, Kodi's direct streaming) keeps using the
// inline download route untouched.
type MediaHandler struct {
	svc       downloadService
	hls       hlsPlayer
	downloads *DownloadHandler
}

func NewMediaHandler(svc downloadService, hls hlsPlayer, downloads *DownloadHandler) *MediaHandler {
	return &MediaHandler{svc: svc, hls: hls, downloads: downloads}
}

type mediaInfoResponse struct {
	Container       string        `json:"container"`
	DurationSeconds float64       `json:"duration_seconds"`
	Tracks          []media.Track `json:"tracks"`
	Error           string        `json:"error,omitempty"`
	HLS             hlsStatus     `json:"hls"`
}

type hlsStatus struct {
	Available      bool   `json:"available"`
	Reason         string `json:"reason,omitempty"`
	VideoCopy      bool   `json:"video_copy"`
	AudioCopy      bool   `json:"audio_copy"`
	SegmentSeconds int    `json:"segment_seconds"`
}

// MediaInfo answers GET /storages/{storageID}/files/media_info/{path}. Parse
// failures are part of the answer (container "unsupported"/"unknown" with a
// reason) rather than errors, so the UI can always explain what it can do.
func (h *MediaHandler) MediaInfo(w http.ResponseWriter, r *http.Request) {
	file, err := h.requestedFile(r)
	if err != nil {
		writeError(w, err)
		return
	}

	resp := mediaInfoResponse{Container: "unknown", Tracks: []media.Track{}, HLS: hlsStatus{SegmentSeconds: h.hls.SegmentSeconds()}}
	info, err := h.hls.MediaInfo(r.Context(), file)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			writeError(w, err)
			return
		}
		if errors.Is(err, media.ErrNotMatroska) {
			resp.Container = "unsupported"
			resp.HLS.Reason = "not a Matroska file"
		} else {
			resp.Error = err.Error()
			resp.HLS.Reason = "could not read the media header"
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	resp.Container = info.Container
	resp.DurationSeconds = info.DurationSeconds
	resp.Tracks = nonNilSlice(info.Tracks)
	resp.HLS = h.hlsStatusFor(info)
	writeJSON(w, http.StatusOK, resp)
}

func (h *MediaHandler) hlsStatusFor(info *media.Info) hlsStatus {
	status := hlsStatus{SegmentSeconds: h.hls.SegmentSeconds()}
	if ok, reason := h.hls.Available(); !ok {
		status.Reason = reason
		return status
	}
	plan, err := media.BuildPlan(info, 0)
	if err != nil {
		status.Reason = err.Error()
		return status
	}
	if info.DurationSeconds <= 0 {
		status.Reason = service.ErrHLSUnknownDuration.Error()
		return status
	}
	status.Available = true
	status.VideoCopy = plan.VideoCopy
	status.AudioCopy = plan.Audio == nil || plan.AudioCopy
	return status
}

// HLS answers GET /storages/{storageID}/files/hls/{path}/{resource}, where
// resource is index.m3u8, init.mp4 or segNNNNN.m4s. The request's query
// string (audio track, access_token for players that cannot send headers)
// is echoed into the playlist so every segment request carries it.
func (h *MediaHandler) HLS(w http.ResponseWriter, r *http.Request) {
	user, storageID, err := storageRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}
	filePath, resource := splitHLSPath(extractWildcardPath(r))
	if filePath == "" || resource == "" {
		writeError(w, domain.ErrBadRequest("file path and hls resource are required"))
		return
	}
	audio, err := parseAudioTrack(r)
	if err != nil {
		writeError(w, err)
		return
	}
	file, err := h.svc.GetFileForDownload(r.Context(), user.ID, storageID, filePath)
	if err != nil {
		writeError(w, err)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	if resource == "index.m3u8" {
		playlist, err := h.hls.Playlist(r.Context(), file, audio, r.URL.RawQuery)
		if err != nil {
			writeError(w, hlsError(err))
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(playlist)
		return
	}

	localPath, err := h.hls.Resource(r.Context(), user.ID, file, audio, resource)
	if err != nil {
		writeError(w, hlsError(err))
		return
	}
	f, err := os.Open(localPath)
	if err != nil {
		writeError(w, domain.ErrInternal("hls segment vanished"))
		return
	}
	defer f.Close()
	if strings.HasSuffix(resource, ".mp4") {
		w.Header().Set("Content-Type", "video/mp4")
	} else {
		w.Header().Set("Content-Type", "video/iso.segment")
	}
	http.ServeContent(w, r, resource, time.Time{}, f)
}

// HLSStop answers POST /storages/{storageID}/files/hls_stop/{path}: the
// player closed, so its ffmpeg can go now instead of at the idle timeout.
func (h *MediaHandler) HLSStop(w http.ResponseWriter, r *http.Request) {
	file, err := h.requestedFile(r)
	if err != nil {
		writeError(w, err)
		return
	}
	h.hls.Stop(GetAuthUser(r.Context()).ID, file.ID)
	w.WriteHeader(http.StatusNoContent)
}

// HLSSource answers GET /internal/hls_source/{token}: the loopback-only
// ranged stream ffmpeg reads a session's file from. The token was minted
// for one session by the HLS service; the user already proved access to the
// file when the session started, so no JWT is involved here.
func (h *MediaHandler) HLSSource(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		writeError(w, domain.ErrForbidden())
		return
	}
	file, ok := h.hls.LookupSource(chi.URLParam(r, "token"))
	if !ok {
		writeError(w, domain.ErrNotFound("hls source"))
		return
	}
	filename := filepath.Base(file.Path)
	h.downloads.serveInline(w, r, servedFile{file: file, filename: filename, contentType: contentTypeForFilename(filename)})
}

func (h *MediaHandler) requestedFile(r *http.Request) (*domain.File, error) {
	user, storageID, err := storageRequest(r)
	if err != nil {
		return nil, err
	}
	filePath := extractWildcardPath(r)
	if filePath == "" {
		return nil, domain.ErrBadRequest("file path is required")
	}
	return h.svc.GetFileForDownload(r.Context(), user.ID, storageID, filePath)
}

// splitHLSPath separates "dir/movie.mkv/seg00001.m4s" into the storage path
// and the HLS resource name.
func splitHLSPath(full string) (filePath, resource string) {
	full = strings.TrimSuffix(full, "/")
	dir, name := path.Split(full)
	return strings.TrimSuffix(dir, "/"), name
}

func parseAudioTrack(r *http.Request) (uint64, error) {
	raw := r.URL.Query().Get("audio")
	if raw == "" {
		return 0, nil
	}
	audio, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, domain.ErrBadRequest("invalid audio track")
	}
	return audio, nil
}

// hlsError maps pipeline failures to HTTP errors the player can act on.
func hlsError(err error) error {
	var appErr *domain.AppError
	switch {
	case errors.As(err, &appErr), errors.Is(err, context.Canceled):
		return err
	case errors.Is(err, service.ErrHLSUnavailable):
		return &domain.AppError{Code: http.StatusServiceUnavailable, Message: err.Error()}
	case errors.Is(err, media.ErrAudioTrackNotFound), errors.Is(err, media.ErrNoVideoTrack),
		errors.Is(err, media.ErrNotMatroska), errors.Is(err, media.ErrHeaderIncomplete),
		errors.Is(err, service.ErrHLSUnknownDuration):
		return domain.ErrBadRequest(err.Error())
	}
	slog.Error("hls playback failed", "err", err)
	return domain.ErrInternal("hls playback failed: " + err.Error())
}

func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
