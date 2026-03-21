package domain

import (
	"errors"
	"fmt"
	"net/http"
)

// Sentinel errors for Telegram and crypto operations.
var (
	ErrTelegramGetFileFailed = errors.New("telegram getFile failed")
	ErrTelegramFileTooBig    = errors.New("telegram file too big for Bot API")
	ErrDecryptionFailed      = errors.New("chunk decryption failed")
	ErrTelegramResolveFailed = errors.New("telegram file_id resolution failed")
	ErrDownloadInterrupted   = errors.New("telegram download stream interrupted")
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

func ErrUnauthorized(msg string) *AppError {
	return &AppError{Code: http.StatusUnauthorized, Message: msg}
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
