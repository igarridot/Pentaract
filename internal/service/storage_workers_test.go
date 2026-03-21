package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
)

type fakeStorageWorkersRepo struct{}

func (f *fakeStorageWorkersRepo) Create(ctx context.Context, name string, userID uuid.UUID, token string, storageID *uuid.UUID) (*domain.StorageWorker, error) {
	return &domain.StorageWorker{Name: name, Token: token, UserID: userID, StorageID: storageID}, nil
}
func (f *fakeStorageWorkersRepo) List(ctx context.Context, userID uuid.UUID) ([]domain.StorageWorker, error) {
	return []domain.StorageWorker{{Name: "w1", UserID: userID}}, nil
}
func (f *fakeStorageWorkersRepo) Update(ctx context.Context, id, userID uuid.UUID, name string, storageID *uuid.UUID) (*domain.StorageWorker, error) {
	return &domain.StorageWorker{ID: id, Name: name, UserID: userID, StorageID: storageID}, nil
}
func (f *fakeStorageWorkersRepo) Delete(ctx context.Context, id, userID uuid.UUID) error { return nil }
func (f *fakeStorageWorkersRepo) HasWorkers(ctx context.Context, storageID uuid.UUID) (bool, error) {
	return true, nil
}

func TestStorageWorkersService(t *testing.T) {
	svc := NewStorageWorkersService(&fakeStorageWorkersRepo{})
	userID := uuid.New()
	storageID := uuid.New()

	if _, err := svc.Create(context.Background(), "", userID, "t", &storageID); err == nil {
		t.Fatalf("expected bad request for empty name")
	}
	if _, err := svc.Create(context.Background(), "n", userID, "", &storageID); err == nil {
		t.Fatalf("expected bad request for empty token")
	}
	if _, err := svc.Create(context.Background(), "n", userID, "t", &storageID); err != nil {
		t.Fatalf("unexpected create error: %v", err)
	}
	if _, err := svc.List(context.Background(), userID); err != nil {
		t.Fatalf("unexpected list error: %v", err)
	}
	if _, err := svc.Update(context.Background(), uuid.New(), userID, "", &storageID); err == nil {
		t.Fatalf("expected bad request for empty name")
	}
	if _, err := svc.Update(context.Background(), uuid.New(), userID, "ok", &storageID); err != nil {
		t.Fatalf("unexpected update error: %v", err)
	}
	if err := svc.Delete(context.Background(), uuid.New(), userID); err != nil {
		t.Fatalf("unexpected delete error: %v", err)
	}
	if has, err := svc.HasWorkers(context.Background(), storageID); err != nil || !has {
		t.Fatalf("unexpected hasWorkers result: %v %v", has, err)
	}
}
