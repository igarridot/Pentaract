package handler

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/service"
)

type StorageWorkersHandler struct {
	svc storageWorkersService
}

type storageWorkersService interface {
	Create(ctx context.Context, name string, userID uuid.UUID, token string, storageID *uuid.UUID) (*domain.StorageWorker, error)
	List(ctx context.Context, userID uuid.UUID) ([]domain.StorageWorker, error)
	Update(ctx context.Context, id, userID uuid.UUID, name string, storageID *uuid.UUID) (*domain.StorageWorker, error)
	Delete(ctx context.Context, id, userID uuid.UUID) error
	HasWorkers(ctx context.Context, storageID uuid.UUID) (bool, error)
}

func NewStorageWorkersHandler(svc *service.StorageWorkersService) *StorageWorkersHandler {
	return NewStorageWorkersHandlerWithService(svc)
}

func NewStorageWorkersHandlerWithService(svc storageWorkersService) *StorageWorkersHandler {
	return &StorageWorkersHandler{svc: svc}
}

type createWorkerRequest struct {
	Name      string  `json:"name"`
	Token     string  `json:"token"`
	StorageID *string `json:"storage_id"`
}

type updateWorkerRequest struct {
	Name      string  `json:"name"`
	StorageID *string `json:"storage_id"`
}

// parseOptionalUUID parses a *string into a *uuid.UUID, returning nil for empty/nil.
func parseOptionalUUID(s *string) (*uuid.UUID, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return nil, domain.ErrBadRequest("invalid storage_id")
	}
	return &id, nil
}

func (h *StorageWorkersHandler) Create(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())

	var req createWorkerRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, err)
		return
	}

	storageID, err := parseOptionalUUID(req.StorageID)
	if err != nil {
		writeError(w, err)
		return
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

	writeJSON(w, http.StatusOK, nonNilSlice(workers))
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

	storageID, err := parseOptionalUUID(req.StorageID)
	if err != nil {
		writeError(w, err)
		return
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
