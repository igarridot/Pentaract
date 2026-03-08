package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/service"
)

type mockAuthService struct {
	loginFn func(ctx context.Context, email, pass string) (*service.LoginResponse, error)
}

func (m *mockAuthService) Login(ctx context.Context, email, pass string) (*service.LoginResponse, error) {
	return m.loginFn(ctx, email, pass)
}

func TestAuthHandlerLogin(t *testing.T) {
	h := NewAuthHandlerWithService(&mockAuthService{
		loginFn: func(ctx context.Context, email, pass string) (*service.LoginResponse, error) {
			return &service.LoginResponse{AccessToken: "token", TokenType: "bearer", ExpiresIn: 1}, nil
		},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(`{"email":"u@example.com","password":"x"}`))
	h.Login(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuthHandlerLoginErrors(t *testing.T) {
	h := NewAuthHandlerWithService(&mockAuthService{
		loginFn: func(ctx context.Context, email, pass string) (*service.LoginResponse, error) {
			return nil, domain.ErrUnauthorized("not authenticated")
		},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(`{"email":"u@example.com","password":"x"}`))
	h.Login(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(`{bad`))
	h.Login(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
