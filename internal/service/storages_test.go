package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
)

type fakeStoragesRepo struct {
	createFn func(ctx context.Context, name string, chatID int64) (*domain.Storage, error)
	listFn   func(ctx context.Context, userID uuid.UUID) ([]domain.StorageWithInfo, error)
	getByID  func(ctx context.Context, id uuid.UUID) (*domain.Storage, error)
	deleteFn func(ctx context.Context, id uuid.UUID) error
}

func (f *fakeStoragesRepo) Create(ctx context.Context, name string, chatID int64) (*domain.Storage, error) {
	return f.createFn(ctx, name, chatID)
}
func (f *fakeStoragesRepo) List(ctx context.Context, userID uuid.UUID) ([]domain.StorageWithInfo, error) {
	return f.listFn(ctx, userID)
}
func (f *fakeStoragesRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Storage, error) {
	return f.getByID(ctx, id)
}
func (f *fakeStoragesRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return f.deleteFn(ctx, id)
}

type fakeStoragesAccessRepo struct {
	hasAccessFn      func(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error)
	createOrUpdateFn func(ctx context.Context, userID, storageID uuid.UUID, accessType domain.AccessType) error
}

func (f *fakeStoragesAccessRepo) HasAccess(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error) {
	return f.hasAccessFn(ctx, userID, storageID, requiredLevel)
}
func (f *fakeStoragesAccessRepo) CreateOrUpdate(ctx context.Context, userID, storageID uuid.UUID, accessType domain.AccessType) error {
	return f.createOrUpdateFn(ctx, userID, storageID, accessType)
}

type fakeStoragesFilesRepo struct {
	listChunksByStorageFn func(ctx context.Context, storageID uuid.UUID) ([]domain.FileChunk, error)
}

func (f *fakeStoragesFilesRepo) ListChunksByStorage(ctx context.Context, storageID uuid.UUID) ([]domain.FileChunk, error) {
	return f.listChunksByStorageFn(ctx, storageID)
}

type fakeStoragesManager struct {
	deleteFromTelegramFn func(ctx context.Context, storage domain.Storage, chunks []domain.FileChunk, progress *DeleteProgress) error
}

func (f *fakeStoragesManager) DeleteFromTelegram(ctx context.Context, storage domain.Storage, chunks []domain.FileChunk, progress *DeleteProgress) error {
	return f.deleteFromTelegramFn(ctx, storage, chunks, progress)
}

func TestStoragesService(t *testing.T) {
	caller := uuid.New()
	storageID := uuid.New()
	stRepo := &fakeStoragesRepo{
		createFn: func(ctx context.Context, name string, chatID int64) (*domain.Storage, error) {
			return &domain.Storage{ID: storageID, Name: name, ChatID: chatID}, nil
		},
		listFn: func(ctx context.Context, userID uuid.UUID) ([]domain.StorageWithInfo, error) {
			return []domain.StorageWithInfo{{ID: storageID, Name: "S"}}, nil
		},
		getByID: func(ctx context.Context, id uuid.UUID) (*domain.Storage, error) {
			return &domain.Storage{ID: id, Name: "S"}, nil
		},
		deleteFn: func(ctx context.Context, id uuid.UUID) error { return nil },
	}
	accRepo := &fakeStoragesAccessRepo{
		hasAccessFn: func(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error) {
			return true, nil
		},
		createOrUpdateFn: func(ctx context.Context, userID, storageID uuid.UUID, accessType domain.AccessType) error { return nil },
	}
	filesRepo := &fakeStoragesFilesRepo{
		listChunksByStorageFn: func(ctx context.Context, storageID uuid.UUID) ([]domain.FileChunk, error) {
			return []domain.FileChunk{{TelegramMessageID: 1}}, nil
		},
	}
	manager := &fakeStoragesManager{
		deleteFromTelegramFn: func(ctx context.Context, storage domain.Storage, chunks []domain.FileChunk, progress *DeleteProgress) error {
			return nil
		},
	}
	svc := NewStoragesService(stRepo, accRepo, filesRepo, manager)

	if _, err := svc.Create(context.Background(), caller, "", 1); err == nil {
		t.Fatalf("expected bad request on empty storage name")
	}
	if _, err := svc.Create(context.Background(), caller, "S", 1); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if _, err := svc.List(context.Background(), caller); err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if _, err := svc.Get(context.Background(), caller, storageID); err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if err := svc.Delete(context.Background(), caller, storageID, nil); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
}

func TestStoragesServiceForbidden(t *testing.T) {
	svc := NewStoragesService(
		&fakeStoragesRepo{
			createFn: func(ctx context.Context, name string, chatID int64) (*domain.Storage, error) { return nil, nil },
			listFn:   func(ctx context.Context, userID uuid.UUID) ([]domain.StorageWithInfo, error) { return nil, nil },
			getByID:  func(ctx context.Context, id uuid.UUID) (*domain.Storage, error) { return &domain.Storage{}, nil },
			deleteFn: func(ctx context.Context, id uuid.UUID) error { return nil },
		},
		&fakeStoragesAccessRepo{
			hasAccessFn: func(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error) {
				return false, nil
			},
			createOrUpdateFn: func(ctx context.Context, userID, storageID uuid.UUID, accessType domain.AccessType) error { return nil },
		},
		&fakeStoragesFilesRepo{listChunksByStorageFn: func(ctx context.Context, storageID uuid.UUID) ([]domain.FileChunk, error) { return nil, nil }},
		&fakeStoragesManager{deleteFromTelegramFn: func(ctx context.Context, storage domain.Storage, chunks []domain.FileChunk, progress *DeleteProgress) error {
			return nil
		}},
	)

	if _, err := svc.Get(context.Background(), uuid.New(), uuid.New()); err == nil {
		t.Fatalf("expected forbidden on get")
	}
	if err := svc.Delete(context.Background(), uuid.New(), uuid.New(), nil); err == nil {
		t.Fatalf("expected forbidden on delete")
	}
}

func TestStoragesServiceErrorBranches(t *testing.T) {
	caller := uuid.New()
	storageID := uuid.New()

	svc := NewStoragesService(
		&fakeStoragesRepo{
			createFn: func(ctx context.Context, name string, chatID int64) (*domain.Storage, error) {
				return &domain.Storage{ID: storageID, Name: name}, nil
			},
			listFn: func(ctx context.Context, userID uuid.UUID) ([]domain.StorageWithInfo, error) { return nil, nil },
			getByID: func(ctx context.Context, id uuid.UUID) (*domain.Storage, error) {
				return nil, errors.New("storage get fail")
			},
			deleteFn: func(ctx context.Context, id uuid.UUID) error {
				return errors.New("delete fail")
			},
		},
		&fakeStoragesAccessRepo{
			hasAccessFn: func(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error) {
				return true, nil
			},
			createOrUpdateFn: func(ctx context.Context, userID, storageID uuid.UUID, accessType domain.AccessType) error {
				return errors.New("grant fail")
			},
		},
		&fakeStoragesFilesRepo{listChunksByStorageFn: func(ctx context.Context, storageID uuid.UUID) ([]domain.FileChunk, error) {
			return nil, errors.New("chunks fail")
		}},
		&fakeStoragesManager{deleteFromTelegramFn: func(ctx context.Context, storage domain.Storage, chunks []domain.FileChunk, progress *DeleteProgress) error {
			return errors.New("telegram fail")
		}},
	)

	if _, err := svc.Create(context.Background(), caller, "S", 1); err == nil {
		t.Fatalf("expected create grant error")
	}
	if _, err := svc.Get(context.Background(), caller, storageID); err == nil {
		t.Fatalf("expected get storage repo error")
	}
	if err := svc.Delete(context.Background(), caller, storageID, nil); err == nil {
		t.Fatalf("expected delete storage repo/get/chunks error")
	}
}
