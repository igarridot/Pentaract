package handler

import (
	"net/http"

	"github.com/Dominux/Pentaract/internal/service"
)

type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler(svc *service.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := parseBody(r, &req); err != nil {
		writeError(w, err)
		return
	}

	resp, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
