package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	appjwt "github.com/Dominux/Pentaract/internal/jwt"
)

func makeToken(t *testing.T, secret string) string {
	t.Helper()
	token, err := appjwt.Generate(appjwt.AuthUser{
		ID:    uuid.New(),
		Email: "user@example.com",
	}, time.Minute, secret)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}
	return token
}

func TestAuthMiddlewareAllowsBearerToken(t *testing.T) {
	secret := "secret"
	token := makeToken(t, secret)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if GetAuthUser(r.Context()) == nil {
			t.Fatalf("expected auth user in context")
		}
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/storages", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	AuthMiddleware(secret)(next).ServeHTTP(w, req)

	if !called {
		t.Fatalf("expected next handler to be called")
	}
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}

func TestAuthMiddlewareRejectsInvalidAuthorizationHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/storages", nil)
	req.Header.Set("Authorization", "InvalidHeader")
	w := httptest.NewRecorder()

	AuthMiddleware("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next should not be called")
	})).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddlewareAllowsQueryTokenOnlyForDownloadEndpoints(t *testing.T) {
	secret := "secret"
	token := makeToken(t, secret)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	allowed := httptest.NewRequest(http.MethodGet, "/api/storages/1/files/download/a.txt?access_token="+token, nil)
	w := httptest.NewRecorder()
	AuthMiddleware(secret)(next).ServeHTTP(w, allowed)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for download endpoint, got %d", w.Code)
	}

	blocked := httptest.NewRequest(http.MethodGet, "/api/storages?access_token="+token, nil)
	w = httptest.NewRecorder()
	AuthMiddleware(secret)(next).ServeHTTP(w, blocked)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for non-download endpoint, got %d", w.Code)
	}
}

func TestAuthMiddlewareRejectsMissingOrInvalidToken(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("next should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/storages", nil)
	w := httptest.NewRecorder()
	AuthMiddleware("secret")(next).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing auth, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/storages", nil)
	req.Header.Set("Authorization", "Bearer invalid.token")
	w = httptest.NewRecorder()
	AuthMiddleware("secret")(next).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid token, got %d", w.Code)
	}
}
