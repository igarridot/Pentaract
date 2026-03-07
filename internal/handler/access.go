package handler

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/service"
)

type AccessHandler struct {
	svc *service.AccessService
}

func NewAccessHandler(svc *service.AccessService) *AccessHandler {
	return &AccessHandler{svc: svc}
}

type grantAccessRequest struct {
	Email      string           `json:"email"`
	AccessType domain.AccessType `json:"access_type"`
}

func (h *AccessHandler) Grant(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	storageID, err := parseUUIDParam(r, "storageID")
	if err != nil {
		writeError(w, err)
		return
	}

	var req grantAccessRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, err)
		return
	}

	if err := h.svc.Grant(r.Context(), user.ID, storageID, req.Email, req.AccessType); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *AccessHandler) List(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	storageID, err := parseUUIDParam(r, "storageID")
	if err != nil {
		writeError(w, err)
		return
	}

	users, err := h.svc.List(r.Context(), user.ID, storageID)
	if err != nil {
		writeError(w, err)
		return
	}

	if users == nil {
		users = []domain.UserWithAccess{}
	}
	writeJSON(w, http.StatusOK, users)
}

type revokeAccessRequest struct {
	UserID string `json:"user_id"`
}

func (h *AccessHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	storageID, err := parseUUIDParam(r, "storageID")
	if err != nil {
		writeError(w, err)
		return
	}

	var req revokeAccessRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, err)
		return
	}

	targetUserID, err := uuid.Parse(req.UserID)
	if err != nil {
		writeError(w, domain.ErrBadRequest("invalid user_id"))
		return
	}

	if err := h.svc.Revoke(r.Context(), user.ID, storageID, targetUserID); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
