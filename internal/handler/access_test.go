package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	appjwt "github.com/Dominux/Pentaract/internal/jwt"
)

type mockAccessService struct {
	grantFn               func(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID, email string, accessType domain.AccessType) error
	listFn                func(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID) ([]domain.UserWithAccess, error)
	revokeFn              func(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID, targetUserID uuid.UUID) error
	listGrantCandidatesFn func(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID) ([]domain.User, error)
}

func (m *mockAccessService) Grant(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID, email string, accessType domain.AccessType) error {
	return m.grantFn(ctx, callerID, storageID, email, accessType)
}
func (m *mockAccessService) List(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID) ([]domain.UserWithAccess, error) {
	return m.listFn(ctx, callerID, storageID)
}
func (m *mockAccessService) Revoke(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID, targetUserID uuid.UUID) error {
	return m.revokeFn(ctx, callerID, storageID, targetUserID)
}
func (m *mockAccessService) ListGrantCandidates(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID) ([]domain.User, error) {
	return m.listGrantCandidatesFn(ctx, callerID, storageID)
}

func withStorageReq(method, body string, storageID uuid.UUID) *http.Request {
	req := httptest.NewRequest(method, "/", bytes.NewBufferString(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("storageID", storageID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	return req.WithContext(context.WithValue(req.Context(), authUserKey, &appjwt.AuthUser{ID: uuid.New(), Email: "admin@example.com"}))
}

func TestAccessHandlerFlows(t *testing.T) {
	storageID := uuid.New()
	target := uuid.New()
	h := NewAccessHandlerWithService(&mockAccessService{
		grantFn: func(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID, email string, accessType domain.AccessType) error {
			return nil
		},
		listFn: func(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID) ([]domain.UserWithAccess, error) {
			return []domain.UserWithAccess{{Email: "u@example.com"}}, nil
		},
		revokeFn: func(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID, targetUserID uuid.UUID) error {
			if targetUserID != target {
				t.Fatalf("unexpected target")
			}
			return nil
		},
		listGrantCandidatesFn: func(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID) ([]domain.User, error) {
			return []domain.User{{Email: "c@example.com"}}, nil
		},
	})

	w := httptest.NewRecorder()
	h.Grant(w, withStorageReq(http.MethodPost, `{"email":"u@example.com","access_type":"w"}`, storageID))
	if w.Code != http.StatusCreated {
		t.Fatalf("grant expected 201, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.List(w, withStorageReq(http.MethodGet, "", storageID))
	if w.Code != http.StatusOK {
		t.Fatalf("list expected 200, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.Revoke(w, withStorageReq(http.MethodDelete, `{"user_id":"`+target.String()+`"}`, storageID))
	if w.Code != http.StatusNoContent {
		t.Fatalf("revoke expected 204, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.GrantCandidates(w, withStorageReq(http.MethodGet, "", storageID))
	if w.Code != http.StatusOK {
		t.Fatalf("candidates expected 200, got %d", w.Code)
	}
}

func TestAccessHandlerErrors(t *testing.T) {
	storageID := uuid.New()
	h := NewAccessHandlerWithService(&mockAccessService{
		grantFn: func(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID, email string, accessType domain.AccessType) error {
			return domain.ErrForbidden()
		},
		listFn: func(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID) ([]domain.UserWithAccess, error) {
			return nil, domain.ErrForbidden()
		},
		revokeFn: func(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID, targetUserID uuid.UUID) error {
			return domain.ErrForbidden()
		},
		listGrantCandidatesFn: func(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID) ([]domain.User, error) {
			return nil, domain.ErrForbidden()
		},
	})
	w := httptest.NewRecorder()
	h.Grant(w, withStorageReq(http.MethodPost, `{bad`, storageID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.Revoke(w, withStorageReq(http.MethodDelete, `{"user_id":"invalid"}`, storageID))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.List(w, withStorageReq(http.MethodGet, "", storageID))
	if w.Code != http.StatusForbidden {
		t.Fatalf("list expected 403 on service error, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.GrantCandidates(w, withStorageReq(http.MethodGet, "", storageID))
	if w.Code != http.StatusForbidden {
		t.Fatalf("candidates expected 403 on service error, got %d", w.Code)
	}
}
