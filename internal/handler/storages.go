package handler

import (
	"net/http"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/service"
)

type StoragesHandler struct {
	svc *service.StoragesService
}

func NewStoragesHandler(svc *service.StoragesService) *StoragesHandler {
	return &StoragesHandler{svc: svc}
}

type createStorageRequest struct {
	Name   string `json:"name"`
	ChatID int64  `json:"chat_id"`
}

func (h *StoragesHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())

	var req createStorageRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, err)
		return
	}

	storage, err := h.svc.Create(r.Context(), user.ID, req.Name, req.ChatID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, storage)
}

func (h *StoragesHandler) List(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())

	storages, err := h.svc.List(r.Context(), user.ID)
	if err != nil {
		writeError(w, err)
		return
	}

	if storages == nil {
		storages = []domain.StorageWithInfo{}
	}
	writeJSON(w, http.StatusOK, storages)
}

func (h *StoragesHandler) Get(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	storageID, err := parseUUIDParam(r, "storageID")
	if err != nil {
		writeError(w, err)
		return
	}

	storage, err := h.svc.Get(r.Context(), user.ID, storageID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, storage)
}

func (h *StoragesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	storageID, err := parseUUIDParam(r, "storageID")
	if err != nil {
		writeError(w, err)
		return
	}

	deleteID := r.URL.Query().Get("delete_id")
	var tracker *deleteTracker
	if deleteID != "" {
		tracker = startDeleteTracker(deleteID, storageID)
		defer scheduleDeleteTrackerCleanup(deleteID)
	}

	var progress *service.DeleteProgress
	if tracker != nil {
		progress = tracker.progress
	}

	if err := h.svc.Delete(r.Context(), user.ID, storageID, progress); err != nil {
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
