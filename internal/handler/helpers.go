package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
)

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

func writeError(w http.ResponseWriter, err error) {
	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		writeJSON(w, appErr.Code, appErr)
		return
	}
	log.Printf("ERROR: %v", err)
	writeJSON(w, http.StatusInternalServerError, &domain.AppError{
		Code:    http.StatusInternalServerError,
		Message: "internal server error",
	})
}

func parseUUIDParam(r *http.Request, name string) (uuid.UUID, error) {
	s := chi.URLParam(r, name)
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, domain.ErrBadRequest("invalid " + name)
	}
	return id, nil
}

func parseBody(r *http.Request, dst interface{}) error {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		return domain.ErrBadRequest("invalid request body")
	}
	return nil
}
