package domain

import (
	"net/http"
	"testing"
)

func TestAppErrorFactories(t *testing.T) {
	tests := []struct {
		name string
		err  *AppError
		code int
		msg  string
	}{
		{"already exists", ErrAlreadyExists("storage"), http.StatusConflict, "storage already exists"},
		{"not found", ErrNotFound("file"), http.StatusNotFound, "file not found"},
		{"not authenticated", ErrNotAuthenticated(), http.StatusUnauthorized, "not authenticated"},
		{"forbidden", ErrForbidden(), http.StatusForbidden, "forbidden"},
		{"bad request", ErrBadRequest("bad"), http.StatusBadRequest, "bad"},
		{"internal", ErrInternal("boom"), http.StatusInternalServerError, "boom"},
		{"no workers", ErrNoWorkers(), http.StatusBadRequest, "no storage workers available for this storage"},
		{"self access", ErrSelfAccess(), http.StatusBadRequest, "cannot change own access"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Code != tt.code {
				t.Fatalf("expected code %d, got %d", tt.code, tt.err.Code)
			}
			if tt.err.Message != tt.msg {
				t.Fatalf("expected message %q, got %q", tt.msg, tt.err.Message)
			}
			if tt.err.Error() != tt.msg {
				t.Fatalf("expected Error() %q, got %q", tt.msg, tt.err.Error())
			}
		})
	}
}
