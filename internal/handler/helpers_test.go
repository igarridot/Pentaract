package handler

import (
	"context"
	"io"
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

func TestParseUUIDQuery(t *testing.T) {
	valid := "ddeb27fb-d9a0-4624-be4d-4615062daed4"

	id, err := parseUUIDQuery(httptest.NewRequest(http.MethodGet, "/?user_id="+valid, nil), "user_id")
	if err != nil || id.String() != valid {
		t.Fatalf("parseUUIDQuery() = %s, %v", id, err)
	}

	tests := map[string]string{
		"/":                   "user_id is required",
		"/?user_id=nope":      "invalid user_id",
		"/?other_id=" + valid: "user_id is required",
	}
	for target, wantMsg := range tests {
		_, err := parseUUIDQuery(httptest.NewRequest(http.MethodGet, target, nil), "user_id")
		appErr, ok := err.(*domain.AppError)
		if !ok || appErr.Code != http.StatusBadRequest || appErr.Message != wantMsg {
			t.Fatalf("parseUUIDQuery(%q) error = %#v, want bad request %q", target, err, wantMsg)
		}
	}
}

func TestParseBoolQuery(t *testing.T) {
	tests := []struct {
		target  string
		want    bool
		wantErr bool
	}{
		{target: "/", want: false},
		{target: "/?force_delete=1", want: true},
		{target: "/?force_delete=false", want: false},
		{target: "/?force_delete=maybe", wantErr: true},
	}
	for _, tt := range tests {
		got, err := parseBoolQuery(httptest.NewRequest(http.MethodGet, tt.target, nil), "force_delete")
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Fatalf("parseBoolQuery(%q) = %v, %v; want %v, err=%v", tt.target, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestStorageJSONRequest(t *testing.T) {
	type req struct {
		Name string `json:"name"`
	}
	storageID := "ddeb27fb-d9a0-4624-be4d-4615062daed4"

	r := reqWithURLParam("storageID", storageID)
	r.Body = io.NopCloser(strings.NewReader(`{"name":"ok"}`))
	var parsed req
	_, gotID, err := storageJSONRequest(r, &parsed)
	if err != nil || gotID.String() != storageID || parsed.Name != "ok" {
		t.Fatalf("storageJSONRequest() = %s, %+v, %v", gotID, parsed, err)
	}

	rBadID := reqWithURLParam("storageID", "nope")
	rBadID.Body = io.NopCloser(strings.NewReader(`{"name":"ok"}`))
	if _, _, err := storageJSONRequest(rBadID, &parsed); err == nil || err.Error() != "invalid storageID" {
		t.Fatalf("expected invalid storage id error, got %v", err)
	}

	rBadBody := reqWithURLParam("storageID", storageID)
	rBadBody.Body = io.NopCloser(strings.NewReader(`{invalid`))
	if _, _, err := storageJSONRequest(rBadBody, &parsed); err == nil || err.Error() != "invalid request body" {
		t.Fatalf("expected invalid body error, got %v", err)
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
