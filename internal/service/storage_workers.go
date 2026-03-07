package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/repository"
)

type StorageWorkersService struct {
	workersRepo *repository.StorageWorkersRepo
}

func NewStorageWorkersService(workersRepo *repository.StorageWorkersRepo) *StorageWorkersService {
	return &StorageWorkersService{workersRepo: workersRepo}
}

func (s *StorageWorkersService) Create(ctx context.Context, name string, userID uuid.UUID, token string, storageID *uuid.UUID) (*domain.StorageWorker, error) {
	if name == "" || token == "" {
		return nil, domain.ErrBadRequest("name and token are required")
	}
	return s.workersRepo.Create(ctx, name, userID, token, storageID)
}

func (s *StorageWorkersService) List(ctx context.Context, userID uuid.UUID) ([]domain.StorageWorker, error) {
	return s.workersRepo.List(ctx, userID)
}

func (s *StorageWorkersService) Update(ctx context.Context, id, userID uuid.UUID, name string, storageID *uuid.UUID) (*domain.StorageWorker, error) {
	if name == "" {
		return nil, domain.ErrBadRequest("name is required")
	}
	return s.workersRepo.Update(ctx, id, userID, name, storageID)
}

func (s *StorageWorkersService) Delete(ctx context.Context, id, userID uuid.UUID) error {
	return s.workersRepo.Delete(ctx, id, userID)
}

func (s *StorageWorkersService) HasWorkers(ctx context.Context, storageID uuid.UUID) (bool, error) {
	return s.workersRepo.HasWorkers(ctx, storageID)
}
