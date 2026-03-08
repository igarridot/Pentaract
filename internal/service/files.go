package service

import (
	"archive/zip"
	"context"
	"io"
	"strings"

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

func (s *FilesService) Upload(ctx context.Context, userID, storageID uuid.UUID, path string, size int64, reader io.Reader, progress *UploadProgress) (*domain.File, error) {
	ok, err := s.accessRepo.HasAccess(ctx, userID, storageID, domain.AccessWrite)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrForbidden()
	}

	file, err := s.filesRepo.CreateFileAnyway(ctx, path, size, storageID)
	if err != nil {
		return nil, err
	}

	if err := s.manager.Upload(ctx, file, reader, progress); err != nil {
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

func (s *FilesService) GetFileForDownload(ctx context.Context, userID, storageID uuid.UUID, path string) (*domain.File, error) {
	ok, err := s.accessRepo.HasAccess(ctx, userID, storageID, domain.AccessRead)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrForbidden()
	}

	return s.filesRepo.GetByPath(ctx, storageID, path)
}

func (s *FilesService) DownloadFileToWriter(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress) error {
	return s.manager.DownloadToWriter(ctx, file, w, progress)
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

func (s *FilesService) Delete(ctx context.Context, userID, storageID uuid.UUID, path string, progress *DeleteProgress) error {
	ok, err := s.accessRepo.HasAccess(ctx, userID, storageID, domain.AccessWrite)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrForbidden()
	}

	// Get chunks before deleting (CASCADE will remove them from DB)
	chunks, err := s.filesRepo.ListChunksByPath(ctx, storageID, path)
	if err != nil {
		return err
	}

	// Get storage info for Telegram deletion
	storage, err := s.manager.storagesRepo.GetByID(ctx, storageID)
	if err != nil {
		return err
	}

	// Delete from Telegram first. DB delete is allowed only after Telegram cleanup succeeds.
	if len(chunks) > 0 {
		if err := s.manager.DeleteFromTelegram(ctx, *storage, chunks, progress); err != nil {
			return err
		}
	}

	// Delete from DB only after Telegram deletion has been confirmed.
	if err := s.filesRepo.Delete(ctx, storageID, path); err != nil {
		return err
	}

	return nil
}

// DownloadDir writes all files under a directory as a zip archive to the given writer.
func (s *FilesService) DownloadDir(ctx context.Context, userID, storageID uuid.UUID, dirPath string, w io.Writer, progress *DownloadProgress) (string, error) {
	ok, err := s.accessRepo.HasAccess(ctx, userID, storageID, domain.AccessRead)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", domain.ErrForbidden()
	}

	if progress != nil {
		totalBytes, totalChunks, err := s.filesRepo.DirStats(ctx, storageID, dirPath)
		if err != nil {
			return "", err
		}
		progress.TotalBytes = totalBytes
		progress.TotalChunks = totalChunks
	}

	files, err := s.filesRepo.ListFilesUnderPath(ctx, storageID, dirPath)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", domain.ErrNotFound("files in directory")
	}

	// Determine zip filename from directory name
	trimmed := strings.TrimSuffix(dirPath, "/")
	dirName := trimmed
	if idx := strings.LastIndex(trimmed, "/"); idx >= 0 {
		dirName = trimmed[idx+1:]
	}
	if dirName == "" {
		dirName = "files"
	}

	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	prefix := dirPath
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	for _, file := range files {
		// Use relative path within the directory
		relPath := strings.TrimPrefix(file.Path, prefix)

		entry, err := zipWriter.Create(relPath)
		if err != nil {
			return "", err
		}

		// Stream chunks directly into the zip entry — no full file buffering
		if err := s.manager.DownloadToWriter(ctx, &file, entry, progress); err != nil {
			return "", err
		}
	}

	return dirName, nil
}

func (s *FilesService) Move(ctx context.Context, userID, storageID uuid.UUID, oldPath, newPath string) error {
	ok, err := s.accessRepo.HasAccess(ctx, userID, storageID, domain.AccessWrite)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrForbidden()
	}

	return s.filesRepo.Move(ctx, storageID, oldPath, newPath)
}

func (s *FilesService) WorkersStatus(storageID uuid.UUID) string {
	if s.manager != nil && s.manager.scheduler != nil && s.manager.scheduler.IsWaiting(storageID) {
		return "waiting_rate_limit"
	}
	return "active"
}
