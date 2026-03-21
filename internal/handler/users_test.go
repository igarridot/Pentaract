package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	appjwt "github.com/Dominux/Pentaract/internal/jwt"
)

type mockUsersService struct {
	registerFn       func(ctx context.Context, email, pass string) (*domain.User, error)
	isAdminFn    func(user *appjwt.AuthUser) bool
	listManagedFn    func(ctx context.Context, caller *appjwt.AuthUser) ([]domain.User, error)
	updatePasswordFn func(ctx context.Context, caller *appjwt.AuthUser, targetUserID uuid.UUID, newPassword string) error
	deleteManagedFn  func(ctx context.Context, caller *appjwt.AuthUser, targetUserID uuid.UUID) error
}

func (m *mockUsersService) Register(ctx context.Context, email, pass string) (*domain.User, error) {
	return m.registerFn(ctx, email, pass)
}
func (m *mockUsersService) IsAdmin(user *appjwt.AuthUser) bool {
	return m.isAdminFn(user)
}
func (m *mockUsersService) ListManaged(ctx context.Context, caller *appjwt.AuthUser) ([]domain.User, error) {
	return m.listManagedFn(ctx, caller)
}
func (m *mockUsersService) UpdatePassword(ctx context.Context, caller *appjwt.AuthUser, targetUserID uuid.UUID, newPassword string) error {
	return m.updatePasswordFn(ctx, caller, targetUserID, newPassword)
}
func (m *mockUsersService) DeleteManaged(ctx context.Context, caller *appjwt.AuthUser, targetUserID uuid.UUID) error {
	return m.deleteManagedFn(ctx, caller, targetUserID)
}

func withAuth(r *http.Request, user *appjwt.AuthUser) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), authUserKey, user))
}

func TestUsersHandlerRegister(t *testing.T) {
	h := NewUsersHandler(&mockUsersService{
		registerFn: func(ctx context.Context, email, pass string) (*domain.User, error) {
			return &domain.User{Email: email}, nil
		},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(`{"email":"u@example.com","password":"x"}`))
	h.Register(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
}

func TestUsersHandlerAdminStatus(t *testing.T) {
	h := NewUsersHandler(&mockUsersService{
		isAdminFn: func(user *appjwt.AuthUser) bool { return true },
	})

	w := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodGet, "/users/admin", nil), &appjwt.AuthUser{Email: "admin@example.com"})
	h.AdminStatus(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestUsersHandlerListManaged(t *testing.T) {
	h := NewUsersHandler(&mockUsersService{
		listManagedFn: func(ctx context.Context, caller *appjwt.AuthUser) ([]domain.User, error) {
			return []domain.User{{Email: "u@example.com"}}, nil
		},
	})
	w := httptest.NewRecorder()
	req := withAuth(httptest.NewRequest(http.MethodGet, "/users/manage", nil), &appjwt.AuthUser{Email: "admin@example.com"})
	h.ListManaged(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestUsersHandlerUpdatePassword(t *testing.T) {
	targetID := uuid.New()
	h := NewUsersHandler(&mockUsersService{
		updatePasswordFn: func(ctx context.Context, caller *appjwt.AuthUser, id uuid.UUID, pass string) error {
			if id != targetID || pass != "newpass" {
				t.Fatalf("unexpected args")
			}
			return nil
		},
	})

	req := httptest.NewRequest(http.MethodPut, "/users/"+targetID.String()+"/password", bytes.NewBufferString(`{"password":"newpass"}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("userID", targetID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = withAuth(req, &appjwt.AuthUser{Email: "admin@example.com"})

	w := httptest.NewRecorder()
	h.UpdatePassword(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}

func TestUsersHandlerDeleteManaged(t *testing.T) {
	targetID := uuid.New()
	h := NewUsersHandler(&mockUsersService{
		deleteManagedFn: func(ctx context.Context, caller *appjwt.AuthUser, id uuid.UUID) error {
			if id != targetID {
				t.Fatalf("unexpected target id")
			}
			return nil
		},
	})

	req := withAuth(httptest.NewRequest(http.MethodDelete, "/users/manage?user_id="+targetID.String(), nil), &appjwt.AuthUser{Email: "admin@example.com"})
	w := httptest.NewRecorder()
	h.DeleteManaged(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}

func TestUsersHandlerDeleteManagedBadRequest(t *testing.T) {
	h := NewUsersHandler(&mockUsersService{
		deleteManagedFn: func(ctx context.Context, caller *appjwt.AuthUser, id uuid.UUID) error { return nil },
	})
	req := withAuth(httptest.NewRequest(http.MethodDelete, "/users/manage", nil), &appjwt.AuthUser{Email: "admin@example.com"})
	w := httptest.NewRecorder()
	h.DeleteManaged(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestUsersHandlerValidationErrors(t *testing.T) {
	h := NewUsersHandler(&mockUsersService{
		registerFn: func(ctx context.Context, email, pass string) (*domain.User, error) { return nil, nil },
		updatePasswordFn: func(ctx context.Context, caller *appjwt.AuthUser, targetUserID uuid.UUID, newPassword string) error {
			return nil
		},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/users", bytes.NewBufferString(`{`))
	h.Register(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("register expected 400 for invalid JSON, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodPut, "/users/not-a-uuid/password", bytes.NewBufferString(`{"password":"x"}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("userID", "not-a-uuid")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = withAuth(req, &appjwt.AuthUser{Email: "admin@example.com"})
	w = httptest.NewRecorder()
	h.UpdatePassword(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("update password expected 400 for invalid uuid, got %d", w.Code)
	}
}

func TestUsersHandlerListManagedError(t *testing.T) {
	h := NewUsersHandler(&mockUsersService{
		listManagedFn: func(ctx context.Context, caller *appjwt.AuthUser) ([]domain.User, error) {
			return nil, domain.ErrForbidden()
		},
	})
	req := withAuth(httptest.NewRequest(http.MethodGet, "/users/manage", nil), &appjwt.AuthUser{Email: "user@example.com"})
	w := httptest.NewRecorder()
	h.ListManaged(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	var payload map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &payload)
	if payload["error"] == "" {
		t.Fatalf("expected error payload")
	}
}
