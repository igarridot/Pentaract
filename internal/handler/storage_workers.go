package handler

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/service"
)

type StorageWorkersHandler struct {
	svc *service.StorageWorkersService
}

func NewStorageWorkersHandler(svc *service.StorageWorkersService) *StorageWorkersHandler {
	return &StorageWorkersHandler{svc: svc}
}

type createWorkerRequest struct {
	Name      string  `json:"name"`
	Token     string  `json:"token"`
	StorageID *string `json:"storage_id"`
}

func (h *StorageWorkersHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())

	var req createWorkerRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, err)
		return
	}

	var storageID *uuid.UUID
	if req.StorageID != nil && *req.StorageID != "" {
		id, err := uuid.Parse(*req.StorageID)
		if err != nil {
			writeError(w, domain.ErrBadRequest("invalid storage_id"))
			return
		}
		storageID = &id
	}

	worker, err := h.svc.Create(r.Context(), req.Name, user.ID, req.Token, storageID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, worker)
}

func (h *StorageWorkersHandler) List(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())

	workers, err := h.svc.List(r.Context(), user.ID)
	if err != nil {
		writeError(w, err)
		return
	}

	if workers == nil {
		workers = []domain.StorageWorker{}
	}
	writeJSON(w, http.StatusOK, workers)
}

type updateWorkerRequest struct {
	Name      string  `json:"name"`
	StorageID *string `json:"storage_id"`
}

func (h *StorageWorkersHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	workerID, err := parseUUIDParam(r, "workerID")
	if err != nil {
		writeError(w, err)
		return
	}

	var req updateWorkerRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, err)
		return
	}

	var storageID *uuid.UUID
	if req.StorageID != nil && *req.StorageID != "" {
		id, err := uuid.Parse(*req.StorageID)
		if err != nil {
			writeError(w, domain.ErrBadRequest("invalid storage_id"))
			return
		}
		storageID = &id
	}

	worker, err := h.svc.Update(r.Context(), workerID, user.ID, req.Name, storageID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, worker)
}

func (h *StorageWorkersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	workerID, err := parseUUIDParam(r, "workerID")
	if err != nil {
		writeError(w, err)
		return
	}

	if err := h.svc.Delete(r.Context(), workerID, user.ID); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *StorageWorkersHandler) HasWorkers(w http.ResponseWriter, r *http.Request) {
	storageIDStr := r.URL.Query().Get("storage_id")
	if storageIDStr == "" {
		writeError(w, domain.ErrBadRequest("storage_id is required"))
		return
	}

	storageID, err := uuid.Parse(storageIDStr)
	if err != nil {
		writeError(w, domain.ErrBadRequest("invalid storage_id"))
		return
	}

	has, err := h.svc.HasWorkers(r.Context(), storageID)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"has_workers": has})
}
