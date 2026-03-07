package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/repository"
)

type FilesService struct {
	filesRepo  *repository.FilesRepo
	accessRepo *repository.AccessRepo
	manager    *StorageManager
}

func NewFilesService(filesRepo *repository.FilesRepo, accessRepo *repository.AccessRepo, manager *StorageManager) *FilesService {
	return &FilesService{
		filesRepo:  filesRepo,
		accessRepo: accessRepo,
		manager:    manager,
	}
}

func (s *FilesService) CreateFolder(ctx context.Context, userID, storageID uuid.UUID, path, folderName string) error {
	ok, err := s.accessRepo.HasAccess(ctx, userID, storageID, domain.AccessWrite)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrForbidden()
	}

	fullPath := folderName
	if path != "" {
		fullPath = path + "/" + folderName
	}

	return s.filesRepo.CreateFolder(ctx, storageID, fullPath)
}

func (s *FilesService) Upload(ctx context.Context, userID, storageID uuid.UUID, path string, data []byte) (*domain.File, error) {
	ok, err := s.accessRepo.HasAccess(ctx, userID, storageID, domain.AccessWrite)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrForbidden()
	}

	file, err := s.filesRepo.CreateFileAnyway(ctx, path, int64(len(data)), storageID)
	if err != nil {
		return nil, err
	}

	if err := s.manager.Upload(ctx, file, data); err != nil {
		return nil, err
	}

	return file, nil
}

func (s *FilesService) Download(ctx context.Context, userID, storageID uuid.UUID, path string) ([]byte, string, error) {
	ok, err := s.accessRepo.HasAccess(ctx, userID, storageID, domain.AccessRead)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return nil, "", domain.ErrForbidden()
	}

	file, err := s.filesRepo.GetByPath(ctx, storageID, path)
	if err != nil {
		return nil, "", err
	}

	data, err := s.manager.Download(ctx, file)
	if err != nil {
		return nil, "", err
	}

	return data, file.Path, nil
}

func (s *FilesService) ListDir(ctx context.Context, userID, storageID uuid.UUID, path string) ([]domain.FSElement, error) {
	ok, err := s.accessRepo.HasAccess(ctx, userID, storageID, domain.AccessRead)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrForbidden()
	}

	return s.filesRepo.ListDir(ctx, storageID, path)
}

func (s *FilesService) Search(ctx context.Context, userID, storageID uuid.UUID, basePath, searchPath string) ([]domain.SearchFSElement, error) {
	ok, err := s.accessRepo.HasAccess(ctx, userID, storageID, domain.AccessRead)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrForbidden()
	}

	return s.filesRepo.Search(ctx, storageID, basePath, searchPath)
}

func (s *FilesService) Delete(ctx context.Context, userID, storageID uuid.UUID, path string) error {
	ok, err := s.accessRepo.HasAccess(ctx, userID, storageID, domain.AccessWrite)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrForbidden()
	}

	return s.filesRepo.Delete(ctx, storageID, path)
}

func (s *FilesService) GetFileInfo(ctx context.Context, userID, storageID uuid.UUID, path string) (*domain.File, error) {
	ok, err := s.accessRepo.HasAccess(ctx, userID, storageID, domain.AccessRead)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrForbidden()
	}

	return s.filesRepo.GetByPath(ctx, storageID, path)
}
