package handler

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
)

type saveWatchProgressRequest struct {
	PositionSeconds float64 `json:"position_seconds"`
	DurationSeconds float64 `json:"duration_seconds"`
}

// SaveWatchProgress answers PUT /storages/{storageID}/files/watch_progress/{path}:
// a player (web UI or Kodi) reporting where the user is in a video.
func (h *FilesHandler) SaveWatchProgress(w http.ResponseWriter, r *http.Request) {
	var req saveWatchProgressRequest
	user, storageID, err := storageJSONRequest(r, &req)
	if err != nil {
		writeError(w, err)
		return
	}
	path := extractWildcardPath(r)
	if path == "" {
		writeError(w, domain.ErrBadRequest("file path is required"))
		return
	}

	progress, err := h.watch.Save(r.Context(), user.ID, storageID, path, req.PositionSeconds, req.DurationSeconds)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, progress)
}

type setWatchedRequest struct {
	Watched *bool `json:"watched"`
}

// SetWatched answers PUT /storages/{storageID}/files/watched/{path}: the user
// marking a video as watched or pending by hand.
func (h *FilesHandler) SetWatched(w http.ResponseWriter, r *http.Request) {
	var req setWatchedRequest
	user, storageID, err := storageJSONRequest(r, &req)
	if err != nil {
		writeError(w, err)
		return
	}
	path := extractWildcardPath(r)
	if path == "" || req.Watched == nil {
		writeError(w, domain.ErrBadRequest("file path and watched are required"))
		return
	}

	progress, err := h.watch.SetWatched(r.Context(), user.ID, storageID, path, *req.Watched)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, progress)
}

// annotateWatchProgress adds the user's progress to listed files. A listing
// is still useful without it, so a failure is logged rather than returned.
func (h *FilesHandler) annotateWatchProgress(r *http.Request, userID, storageID uuid.UUID, elements []domain.FSElement) {
	if err := h.watch.Annotate(r.Context(), userID, storageID, elements); err != nil {
		slog.Warn("loading watch progress for listing", "err", err)
	}
}
