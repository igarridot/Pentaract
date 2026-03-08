package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/repository"
)

type StoragesService struct {
	storagesRepo storagesRepository
	accessRepo   storagesAccessRepository
	filesRepo    storagesFilesRepository
	manager      storagesManager
}

type storagesRepository interface {
	Create(ctx context.Context, name string, chatID int64) (*domain.Storage, error)
	List(ctx context.Context, userID uuid.UUID) ([]domain.StorageWithInfo, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Storage, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type storagesAccessRepository interface {
	HasAccess(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error)
	CreateOrUpdate(ctx context.Context, userID, storageID uuid.UUID, accessType domain.AccessType) error
}

type storagesFilesRepository interface {
	ListChunksByStorage(ctx context.Context, storageID uuid.UUID) ([]domain.FileChunk, error)
}

type storagesManager interface {
	DeleteFromTelegram(ctx context.Context, storage domain.Storage, chunks []domain.FileChunk, progress *DeleteProgress) error
}

func NewStoragesService(
	storagesRepo *repository.StoragesRepo,
	accessRepo *repository.AccessRepo,
	filesRepo *repository.FilesRepo,
	manager *StorageManager,
) *StoragesService {
	return NewStoragesServiceWithDeps(storagesRepo, accessRepo, filesRepo, manager)
}

func NewStoragesServiceWithDeps(
	storagesRepo storagesRepository,
	accessRepo storagesAccessRepository,
	filesRepo storagesFilesRepository,
	manager storagesManager,
) *StoragesService {
	return &StoragesService{
		storagesRepo: storagesRepo,
		accessRepo:   accessRepo,
		filesRepo:    filesRepo,
		manager:      manager,
	}
}

func (s *StoragesService) Create(ctx context.Context, userID uuid.UUID, name string, chatID int64) (*domain.Storage, error) {
	if name == "" {
		return nil, domain.ErrBadRequest("name is required")
	}

	storage, err := s.storagesRepo.Create(ctx, name, chatID)
	if err != nil {
		return nil, err
	}

	// Grant admin access to creator
	if err := s.accessRepo.CreateOrUpdate(ctx, userID, storage.ID, domain.AccessAdmin); err != nil {
		return nil, err
	}

	return storage, nil
}

func (s *StoragesService) List(ctx context.Context, userID uuid.UUID) ([]domain.StorageWithInfo, error) {
	return s.storagesRepo.List(ctx, userID)
}

func (s *StoragesService) Get(ctx context.Context, userID uuid.UUID, storageID uuid.UUID) (*domain.Storage, error) {
	if err := requireStorageAccess(ctx, s.accessRepo, userID, storageID, domain.AccessRead); err != nil {
		return nil, err
	}

	return s.storagesRepo.GetByID(ctx, storageID)
}

func (s *StoragesService) Delete(ctx context.Context, userID uuid.UUID, storageID uuid.UUID, progress *DeleteProgress) error {
	if err := requireStorageAccess(ctx, s.accessRepo, userID, storageID, domain.AccessAdmin); err != nil {
		return err
	}

	storage, err := s.storagesRepo.GetByID(ctx, storageID)
	if err != nil {
		return err
	}

	chunks, err := s.filesRepo.ListChunksByStorage(ctx, storageID)
	if err != nil {
		return err
	}
	if len(chunks) > 0 {
		// Telegram cleanup must finish successfully before DB deletion.
		if err := s.manager.DeleteFromTelegram(ctx, *storage, chunks, progress); err != nil {
			return err
		}
	}

	return s.storagesRepo.Delete(ctx, storageID)
}
