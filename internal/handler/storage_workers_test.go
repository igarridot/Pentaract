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

type mockStorageWorkersService struct {
	createFn     func(ctx context.Context, name string, userID uuid.UUID, token string, storageID *uuid.UUID) (*domain.StorageWorker, error)
	listFn       func(ctx context.Context, userID uuid.UUID) ([]domain.StorageWorker, error)
	updateFn     func(ctx context.Context, id, userID uuid.UUID, name string, storageID *uuid.UUID) (*domain.StorageWorker, error)
	deleteFn     func(ctx context.Context, id, userID uuid.UUID) error
	hasWorkersFn func(ctx context.Context, storageID uuid.UUID) (bool, error)
}

func (m *mockStorageWorkersService) Create(ctx context.Context, name string, userID uuid.UUID, token string, storageID *uuid.UUID) (*domain.StorageWorker, error) {
	return m.createFn(ctx, name, userID, token, storageID)
}
func (m *mockStorageWorkersService) List(ctx context.Context, userID uuid.UUID) ([]domain.StorageWorker, error) {
	return m.listFn(ctx, userID)
}
func (m *mockStorageWorkersService) Update(ctx context.Context, id, userID uuid.UUID, name string, storageID *uuid.UUID) (*domain.StorageWorker, error) {
	return m.updateFn(ctx, id, userID, name, storageID)
}
func (m *mockStorageWorkersService) Delete(ctx context.Context, id, userID uuid.UUID) error {
	return m.deleteFn(ctx, id, userID)
}
func (m *mockStorageWorkersService) HasWorkers(ctx context.Context, storageID uuid.UUID) (bool, error) {
	return m.hasWorkersFn(ctx, storageID)
}

func makeWorkerReq(method, body string, params map[string]string) *http.Request {
	req := httptest.NewRequest(method, "/", bytes.NewBufferString(body))
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	return req.WithContext(context.WithValue(req.Context(), authUserKey, &appjwt.AuthUser{ID: uuid.New(), Email: "u@example.com"}))
}

func TestStorageWorkersHandlerFlows(t *testing.T) {
	h := NewStorageWorkersHandler(&mockStorageWorkersService{
		createFn: func(ctx context.Context, name string, userID uuid.UUID, token string, storageID *uuid.UUID) (*domain.StorageWorker, error) {
			return &domain.StorageWorker{Name: name}, nil
		},
		listFn: func(ctx context.Context, userID uuid.UUID) ([]domain.StorageWorker, error) {
			return []domain.StorageWorker{{Name: "w"}}, nil
		},
		updateFn: func(ctx context.Context, id, userID uuid.UUID, name string, storageID *uuid.UUID) (*domain.StorageWorker, error) {
			return &domain.StorageWorker{ID: id, Name: name}, nil
		},
		deleteFn: func(ctx context.Context, id, userID uuid.UUID) error { return nil },
		hasWorkersFn: func(ctx context.Context, storageID uuid.UUID) (bool, error) {
			return true, nil
		},
	})

	w := httptest.NewRecorder()
	h.Create(w, makeWorkerReq(http.MethodPost, `{"name":"w","token":"t"}`, nil))
	if w.Code != http.StatusCreated {
		t.Fatalf("create expected 201, got %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.List(w, makeWorkerReq(http.MethodGet, "", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list expected 200, got %d", w.Code)
	}
	id := uuid.New()
	w = httptest.NewRecorder()
	h.Update(w, makeWorkerReq(http.MethodPut, `{"name":"w2"}`, map[string]string{"workerID": id.String()}))
	if w.Code != http.StatusOK {
		t.Fatalf("update expected 200, got %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.Delete(w, makeWorkerReq(http.MethodDelete, "", map[string]string{"workerID": id.String()}))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete expected 204, got %d", w.Code)
	}
	w = httptest.NewRecorder()
	req := makeWorkerReq(http.MethodGet, "", nil)
	q := req.URL.Query()
	q.Set("storage_id", uuid.New().String())
	req.URL.RawQuery = q.Encode()
	h.HasWorkers(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("hasWorkers expected 200, got %d", w.Code)
	}
}

func TestStorageWorkersParseOptionalUUID(t *testing.T) {
	if id, err := parseOptionalUUID(nil); err != nil || id != nil {
		t.Fatalf("expected nil,nil for nil input")
	}
	empty := ""
	if id, err := parseOptionalUUID(&empty); err != nil || id != nil {
		t.Fatalf("expected nil,nil for empty input")
	}
	bad := "bad"
	if _, err := parseOptionalUUID(&bad); err == nil {
		t.Fatalf("expected bad request for invalid uuid")
	}
}

func TestStorageWorkersHandlerValidationErrors(t *testing.T) {
	h := NewStorageWorkersHandler(&mockStorageWorkersService{
		createFn: func(ctx context.Context, name string, userID uuid.UUID, token string, storageID *uuid.UUID) (*domain.StorageWorker, error) {
			return nil, nil
		},
		listFn: func(ctx context.Context, userID uuid.UUID) ([]domain.StorageWorker, error) { return nil, nil },
		updateFn: func(ctx context.Context, id, userID uuid.UUID, name string, storageID *uuid.UUID) (*domain.StorageWorker, error) {
			return nil, nil
		},
		deleteFn:     func(ctx context.Context, id, userID uuid.UUID) error { return nil },
		hasWorkersFn: func(ctx context.Context, storageID uuid.UUID) (bool, error) { return false, nil },
	})

	w := httptest.NewRecorder()
	h.Create(w, makeWorkerReq(http.MethodPost, `{`, nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("create expected 400 for invalid JSON, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.Update(w, makeWorkerReq(http.MethodPut, `{"name":"x"}`, map[string]string{"workerID": "bad"}))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("update expected 400 for invalid workerID, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.Delete(w, makeWorkerReq(http.MethodDelete, "", map[string]string{"workerID": "bad"}))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("delete expected 400 for invalid workerID, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req := makeWorkerReq(http.MethodGet, "", nil)
	q := req.URL.Query()
	q.Set("storage_id", "bad")
	req.URL.RawQuery = q.Encode()
	h.HasWorkers(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("hasWorkers expected 400 for invalid storage_id, got %d", w.Code)
	}
}

func TestStorageWorkersHandlerServiceErrors(t *testing.T) {
	h := NewStorageWorkersHandler(&mockStorageWorkersService{
		createFn: func(ctx context.Context, name string, userID uuid.UUID, token string, storageID *uuid.UUID) (*domain.StorageWorker, error) {
			return nil, domain.ErrForbidden()
		},
		listFn: func(ctx context.Context, userID uuid.UUID) ([]domain.StorageWorker, error) {
			return nil, domain.ErrForbidden()
		},
		updateFn: func(ctx context.Context, id, userID uuid.UUID, name string, storageID *uuid.UUID) (*domain.StorageWorker, error) {
			return nil, domain.ErrForbidden()
		},
		deleteFn: func(ctx context.Context, id, userID uuid.UUID) error {
			return domain.ErrForbidden()
		},
		hasWorkersFn: func(ctx context.Context, storageID uuid.UUID) (bool, error) {
			return false, domain.ErrForbidden()
		},
	})

	id := uuid.New().String()
	w := httptest.NewRecorder()
	h.Create(w, makeWorkerReq(http.MethodPost, `{"name":"w","token":"t"}`, nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("create expected 403 on service error, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.List(w, makeWorkerReq(http.MethodGet, "", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("list expected 403 on service error, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.Update(w, makeWorkerReq(http.MethodPut, `{"name":"w2"}`, map[string]string{"workerID": id}))
	if w.Code != http.StatusForbidden {
		t.Fatalf("update expected 403 on service error, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	h.Delete(w, makeWorkerReq(http.MethodDelete, "", map[string]string{"workerID": id}))
	if w.Code != http.StatusForbidden {
		t.Fatalf("delete expected 403 on service error, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req := makeWorkerReq(http.MethodGet, "", nil)
	q := req.URL.Query()
	q.Set("storage_id", uuid.New().String())
	req.URL.RawQuery = q.Encode()
	h.HasWorkers(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("hasWorkers expected 403 on service error, got %d", w.Code)
	}
}
