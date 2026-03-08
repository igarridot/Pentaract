package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/Dominux/Pentaract/internal/domain"
)

func reqWithURLParam(name, value string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(name, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestParseUUIDParam(t *testing.T) {
	valid := "ddeb27fb-d9a0-4624-be4d-4615062daed4"

	id, err := parseUUIDParam(reqWithURLParam("id", valid), "id")
	if err != nil {
		t.Fatalf("parseUUIDParam() unexpected error: %v", err)
	}
	if id.String() != valid {
		t.Fatalf("unexpected id: %s", id)
	}

	_, err = parseUUIDParam(reqWithURLParam("id", "invalid"), "id")
	if err == nil {
		t.Fatalf("parseUUIDParam() expected error for invalid uuid")
	}
	if appErr, ok := err.(*domain.AppError); !ok || appErr.Code != http.StatusBadRequest {
		t.Fatalf("expected AppError bad request, got %#v", err)
	}
}

func TestParseBody(t *testing.T) {
	type req struct {
		Name string `json:"name"`
	}

	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"ok"}`))
	var parsed req
	if err := parseBody(r, &parsed); err != nil {
		t.Fatalf("parseBody() unexpected error: %v", err)
	}
	if parsed.Name != "ok" {
		t.Fatalf("unexpected parsed value: %+v", parsed)
	}

	rBad := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{invalid`))
	if err := parseBody(rBad, &parsed); err == nil {
		t.Fatalf("parseBody() expected error for invalid json")
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, domain.ErrForbidden())
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "forbidden") {
		t.Fatalf("expected forbidden message in body, got %q", w.Body.String())
	}

	w = httptest.NewRecorder()
	writeError(w, context.DeadlineExceeded)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "internal server error") {
		t.Fatalf("expected internal message in body, got %q", w.Body.String())
	}
}
