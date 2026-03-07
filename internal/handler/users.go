package handler

import (
	"net/http"

	"github.com/Dominux/Pentaract/internal/service"
)

type UsersHandler struct {
	svc *service.UsersService
}

func NewUsersHandler(svc *service.UsersService) *UsersHandler {
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
