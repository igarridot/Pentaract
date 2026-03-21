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
	"github.com/Dominux/Pentaract/internal/service"
)

type mockStoragesService struct {
	createFn func(ctx context.Context, userID uuid.UUID, name string, chatID int64) (*domain.Storage, error)
	listFn   func(ctx context.Context, userID uuid.UUID) ([]domain.StorageWithInfo, error)
	getFn    func(ctx context.Context, userID uuid.UUID, storageID uuid.UUID) (*domain.Storage, error)
	deleteFn func(ctx context.Context, userID uuid.UUID, storageID uuid.UUID, progress *service.DeleteProgress) error
}

func (m *mockStoragesService) Create(ctx context.Context, userID uuid.UUID, name string, chatID int64) (*domain.Storage, error) {
	return m.createFn(ctx, userID, name, chatID)
}
func (m *mockStoragesService) List(ctx context.Context, userID uuid.UUID) ([]domain.StorageWithInfo, error) {
	return m.listFn(ctx, userID)
}
func (m *mockStoragesService) Get(ctx context.Context, userID uuid.UUID, storageID uuid.UUID) (*domain.Storage, error) {
	return m.getFn(ctx, userID, storageID)
}
func (m *mockStoragesService) Delete(ctx context.Context, userID uuid.UUID, storageID uuid.UUID, progress *service.DeleteProgress) error {
	return m.deleteFn(ctx, userID, storageID, progress)
}

func makeStorageReq(method, body string, storageID string) *http.Request {
	req := httptest.NewRequest(method, "/", bytes.NewBufferString(body))
	rctx := chi.NewRouteContext()
	if storageID != "" {
		rctx.URLParams.Add("storageID", storageID)
	}
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	return req.WithContext(context.WithValue(req.Context(), authUserKey, &appjwt.AuthUser{ID: uuid.New(), Email: "u@example.com"}))
}

func TestStoragesHandlerFlows(t *testing.T) {
	id := uuid.New()
	h := NewStoragesHandler(&mockStoragesService{
		createFn: func(ctx context.Context, userID uuid.UUID, name string, chatID int64) (*domain.Storage, error) {
			return &domain.Storage{ID: id, Name: name}, nil
		},
		listFn: func(ctx context.Context, userID uuid.UUID) ([]domain.StorageWithInfo, error) {
			return []domain.StorageWithInfo{{ID: id, Name: "s"}}, nil
		},
		getFn: func(ctx context.Context, userID uuid.UUID, storageID uuid.UUID) (*domain.Storage, error) {
			return &domain.Storage{ID: storageID, Name: "s"}, nil
		},
		deleteFn: func(ctx context.Context, userID uuid.UUID, storageID uuid.UUID, progress *service.DeleteProgress) error {
			return nil
		},
	})

	w := httptest.NewRecorder()
	h.Create(w, makeStorageReq(http.MethodPost, `{"name":"s","chat_id":1}`, ""))
	if w.Code != http.StatusCreated {
		t.Fatalf("create expected 201, got %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.List(w, makeStorageReq(http.MethodGet, "", ""))
	if w.Code != http.StatusOK {
		t.Fatalf("list expected 200, got %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.Get(w, makeStorageReq(http.MethodGet, "", id.String()))
	if w.Code != http.StatusOK {
		t.Fatalf("get expected 200, got %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.Delete(w, makeStorageReq(http.MethodDelete, "", id.String()))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete expected 204, got %d", w.Code)
	}
}

func TestStoragesHandlerValidationErrors(t *testing.T) {
	h := NewStoragesHandler(&mockStoragesService{
		createFn: func(ctx context.Context, userID uuid.UUID, name string, chatID int64) (*domain.Storage, error) {
			return nil, nil
		},
		listFn: func(ctx context.Context, userID uuid.UUID) ([]domain.StorageWithInfo, error) { return nil, nil },
		getFn: func(ctx context.Context, userID uuid.UUID, storageID uuid.UUID) (*domain.Storage, error) {
			return nil, nil
		},
		deleteFn: func(ctx context.Context, userID uuid.UUID, storageID uuid.UUID, progress *service.DeleteProgress) error {
			return nil
		},
	})

	w := httptest.NewRecorder()
	h.Create(w, makeStorageReq(http.MethodPost, `{`, ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("create expected 400 for invalid JSON, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.Get(w, makeStorageReq(http.MethodGet, "", "bad-uuid"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("get expected 400 for bad storageID, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.Delete(w, makeStorageReq(http.MethodDelete, "", "bad-uuid"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("delete expected 400 for bad storageID, got %d", w.Code)
	}
}

func TestStoragesHandlerServiceErrors(t *testing.T) {
	id := uuid.New()
	h := NewStoragesHandler(&mockStoragesService{
		createFn: func(ctx context.Context, userID uuid.UUID, name string, chatID int64) (*domain.Storage, error) {
			return nil, domain.ErrForbidden()
		},
		listFn: func(ctx context.Context, userID uuid.UUID) ([]domain.StorageWithInfo, error) {
			return nil, domain.ErrForbidden()
		},
		getFn: func(ctx context.Context, userID uuid.UUID, storageID uuid.UUID) (*domain.Storage, error) {
			return nil, domain.ErrForbidden()
		},
		deleteFn: func(ctx context.Context, userID uuid.UUID, storageID uuid.UUID, progress *service.DeleteProgress) error {
			return domain.ErrForbidden()
		},
	})

	w := httptest.NewRecorder()
	h.Create(w, makeStorageReq(http.MethodPost, `{"name":"s","chat_id":1}`, ""))
	if w.Code != http.StatusForbidden {
		t.Fatalf("create expected 403 on service error, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.List(w, makeStorageReq(http.MethodGet, "", ""))
	if w.Code != http.StatusForbidden {
		t.Fatalf("list expected 403 on service error, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.Get(w, makeStorageReq(http.MethodGet, "", id.String()))
	if w.Code != http.StatusForbidden {
		t.Fatalf("get expected 403 on service error, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.Delete(w, makeStorageReq(http.MethodDelete, "", id.String()))
	if w.Code != http.StatusForbidden {
		t.Fatalf("delete expected 403 on service error, got %d", w.Code)
	}
}
