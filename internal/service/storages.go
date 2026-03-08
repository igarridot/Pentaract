package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/repository"
)

type StoragesService struct {
	storagesRepo *repository.StoragesRepo
	accessRepo   *repository.AccessRepo
	filesRepo    *repository.FilesRepo
	manager      *StorageManager
}

func NewStoragesService(
	storagesRepo *repository.StoragesRepo,
	accessRepo *repository.AccessRepo,
	filesRepo *repository.FilesRepo,
	manager *StorageManager,
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
	ok, err := s.accessRepo.HasAccess(ctx, userID, storageID, domain.AccessRead)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrForbidden()
	}

	return s.storagesRepo.GetByID(ctx, storageID)
}

func (s *StoragesService) Delete(ctx context.Context, userID uuid.UUID, storageID uuid.UUID, progress *DeleteProgress) error {
	ok, err := s.accessRepo.HasAccess(ctx, userID, storageID, domain.AccessAdmin)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrForbidden()
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
