package domain

import (
	"fmt"
	"net/http"
)

type AppError struct {
	Code    int    `json:"-"`
	Message string `json:"error"`
}

func (e *AppError) Error() string {
	return e.Message
}

func ErrAlreadyExists(what string) *AppError {
	return &AppError{Code: http.StatusConflict, Message: fmt.Sprintf("%s already exists", what)}
}

func ErrNotFound(what string) *AppError {
	return &AppError{Code: http.StatusNotFound, Message: fmt.Sprintf("%s not found", what)}
}

func ErrNotAuthenticated() *AppError {
	return &AppError{Code: http.StatusUnauthorized, Message: "not authenticated"}
}

func ErrForbidden() *AppError {
	return &AppError{Code: http.StatusForbidden, Message: "forbidden"}
}

func ErrBadRequest(msg string) *AppError {
	return &AppError{Code: http.StatusBadRequest, Message: msg}
}

func ErrInternal(msg string) *AppError {
	return &AppError{Code: http.StatusInternalServerError, Message: msg}
}

func ErrNoWorkers() *AppError {
	return &AppError{Code: http.StatusBadRequest, Message: "no storage workers available for this storage"}
}

func ErrSelfAccess() *AppError {
	return &AppError{Code: http.StatusBadRequest, Message: "cannot change own access"}
}
