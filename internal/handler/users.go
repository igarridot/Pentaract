package handler

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	appjwt "github.com/Dominux/Pentaract/internal/jwt"
	"github.com/Dominux/Pentaract/internal/service"
)

type UsersHandler struct {
	svc usersService
}

type usersService interface {
	Register(ctx context.Context, email, pass string) (*domain.User, error)
	AdminStatus(user *appjwt.AuthUser) bool
	ListManaged(ctx context.Context, caller *appjwt.AuthUser) ([]domain.User, error)
	UpdatePassword(ctx context.Context, caller *appjwt.AuthUser, targetUserID uuid.UUID, newPassword string) error
	DeleteManaged(ctx context.Context, caller *appjwt.AuthUser, targetUserID uuid.UUID) error
}

func NewUsersHandler(svc *service.UsersService) *UsersHandler {
	return NewUsersHandlerWithService(svc)
}

func NewUsersHandlerWithService(svc usersService) *UsersHandler {
	return &UsersHandler{svc: svc}
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *UsersHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, err)
		return
	}

	user, err := h.svc.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, user)
}

func (h *UsersHandler) AdminStatus(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	writeJSON(w, http.StatusOK, map[string]bool{"is_admin": h.svc.AdminStatus(user)})
}

func (h *UsersHandler) ListManaged(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	users, err := h.svc.ListManaged(r.Context(), user)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, nonNilSlice(users))
}

type updatePasswordRequest struct {
	Password string `json:"password"`
}

func (h *UsersHandler) UpdatePassword(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	targetID, err := parseUUIDParam(r, "userID")
	if err != nil {
		writeError(w, err)
		return
	}

	var req updatePasswordRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, err)
		return
	}

	if err := h.svc.UpdatePassword(r.Context(), user, targetID, req.Password); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *UsersHandler) DeleteManaged(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r.Context())
	targetIDStr := r.URL.Query().Get("user_id")
	if targetIDStr == "" {
		writeError(w, domain.ErrBadRequest("user_id is required"))
		return
	}
	targetID, err := uuid.Parse(targetIDStr)
	if err != nil {
		writeError(w, domain.ErrBadRequest("invalid user_id"))
		return
	}

	if err := h.svc.DeleteManaged(r.Context(), user, targetID); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
