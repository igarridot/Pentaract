package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	appjwt "github.com/Dominux/Pentaract/internal/jwt"
)

func writeJSON(w http.ResponseWriter, status int, data any) {
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
	if errors.Is(err, context.Canceled) {
		// The client went away; nothing is wrong on our side.
		slog.Debug("request cancelled by client", "err", err)
	} else {
		slog.Error("unhandled error", "err", err)
	}
	writeJSON(w, http.StatusInternalServerError, &domain.AppError{
		Code:    http.StatusInternalServerError,
		Message: "internal server error",
	})
}

// parseUUID parses s, reporting a bad request that names the field.
func parseUUID(s, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, domain.ErrBadRequest("invalid " + name)
	}
	return id, nil
}

func parseUUIDParam(r *http.Request, name string) (uuid.UUID, error) {
	return parseUUID(chi.URLParam(r, name), name)
}

// parseUUIDQuery parses a required UUID query parameter.
func parseUUIDQuery(r *http.Request, name string) (uuid.UUID, error) {
	s := r.URL.Query().Get(name)
	if s == "" {
		return uuid.Nil, domain.ErrBadRequest(name + " is required")
	}
	return parseUUID(s, name)
}

// parseBoolQuery parses an optional boolean query parameter; absent means false.
func parseBoolQuery(r *http.Request, name string) (bool, error) {
	s := r.URL.Query().Get(name)
	if s == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(s)
	if err != nil {
		return false, domain.ErrBadRequest("invalid " + name + " value")
	}
	return value, nil
}

// storageRequest returns the authenticated user and the storage addressed by
// the route, the preamble of every storage-scoped handler.
func storageRequest(r *http.Request) (*appjwt.AuthUser, uuid.UUID, error) {
	storageID, err := parseUUIDParam(r, "storageID")
	if err != nil {
		return nil, uuid.Nil, err
	}
	return GetAuthUser(r.Context()), storageID, nil
}

// storageJSONRequest is storageRequest for handlers that also take a JSON
// body, decoded into dst.
func storageJSONRequest(r *http.Request, dst any) (*appjwt.AuthUser, uuid.UUID, error) {
	user, storageID, err := storageRequest(r)
	if err != nil {
		return nil, uuid.Nil, err
	}
	if err := parseBody(r, dst); err != nil {
		return nil, uuid.Nil, err
	}
	return user, storageID, nil
}

func parseBody(r *http.Request, dst any) error {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		return domain.ErrBadRequest("invalid request body")
	}
	return nil
}

func nonNilSlice[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}
