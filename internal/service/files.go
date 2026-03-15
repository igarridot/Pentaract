package service

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/pathutil"
	"github.com/Dominux/Pentaract/internal/repository"
)

type FilesService struct {
	filesRepo    filesRepository
	accessRepo   filesAccessRepository
	manager      filesManager
	storagesRepo filesStorageRepository
	scheduler    *WorkerScheduler
}

type filesRepository interface {
	CreateFolder(ctx context.Context, storageID uuid.UUID, path string) error
	CreateFileAnyway(ctx context.Context, path string, size int64, storageID uuid.UUID) (*domain.File, error)
	CreateFileIfNotExists(ctx context.Context, path string, size int64, storageID uuid.UUID) (*domain.File, bool, error)
	GetByPath(ctx context.Context, storageID uuid.UUID, path string) (*domain.File, error)
	ListDir(ctx context.Context, storageID uuid.UUID, path string) ([]domain.FSElement, error)
	Search(ctx context.Context, storageID uuid.UUID, basePath, searchPath string) ([]domain.FSElement, error)
	ListChunksByPath(ctx context.Context, storageID uuid.UUID, path string) ([]domain.FileChunk, error)
	Delete(ctx context.Context, storageID uuid.UUID, path string) error
	DirStats(ctx context.Context, storageID uuid.UUID, path string) (int64, int64, error)
	ListFilesUnderPath(ctx context.Context, storageID uuid.UUID, path string) ([]domain.File, error)
	Move(ctx context.Context, storageID uuid.UUID, oldPath, newPath string) error
}

const (
	UploadConflictKeepBoth = "keep_both"
	UploadConflictSkip     = "skip"
)

type filesAccessRepository interface {
	HasAccess(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error)
}

type filesManager interface {
	Upload(ctx context.Context, file *domain.File, reader io.Reader, progress *UploadProgress) error
	DownloadToWriter(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress) error
	StreamToWriter(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress) error
	ExactFileSize(ctx context.Context, file *domain.File) (int64, error)
	DownloadRangeToWriter(ctx context.Context, file *domain.File, w io.Writer, start, end, totalSize int64, progress *DownloadProgress) error
	DeleteFromTelegram(ctx context.Context, storage domain.Storage, chunks []domain.FileChunk, progress *DeleteProgress) error
}

type filesStorageRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Storage, error)
}

func NewFilesService(filesRepo *repository.FilesRepo, accessRepo *repository.AccessRepo, manager *StorageManager) *FilesService {
	return NewFilesServiceWithDeps(filesRepo, accessRepo, manager, manager.storagesRepo, manager.scheduler)
}

func NewFilesServiceWithDeps(filesRepo filesRepository, accessRepo filesAccessRepository, manager filesManager, storagesRepo filesStorageRepository, scheduler *WorkerScheduler) *FilesService {
	return &FilesService{
		filesRepo:    filesRepo,
		accessRepo:   accessRepo,
		manager:      manager,
		storagesRepo: storagesRepo,
		scheduler:    scheduler,
	}
}

func (s *FilesService) CreateFolder(ctx context.Context, userID, storageID uuid.UUID, path, folderName string) error {
	if err := requireStorageAccess(ctx, s.accessRepo, userID, storageID, domain.AccessWrite); err != nil {
		return err
	}

	name := strings.Trim(folderName, "/")
	if name == "" {
		return domain.ErrBadRequest("folder_name is required")
	}
	if strings.Contains(name, "/") {
		return domain.ErrBadRequest("folder_name cannot contain /")
	}

	return s.filesRepo.CreateFolder(ctx, storageID, pathutil.Join(path, name))
}

func (s *FilesService) Upload(ctx context.Context, userID, storageID uuid.UUID, path string, size int64, reader io.Reader, progress *UploadProgress, onConflict string) (*domain.File, bool, error) {
	if err := requireStorageAccess(ctx, s.accessRepo, userID, storageID, domain.AccessWrite); err != nil {
		return nil, false, err
	}

	if onConflict == "" {
		onConflict = UploadConflictKeepBoth
	}

	var (
		file    *domain.File
		skipped bool
		err     error
	)
	switch onConflict {
	case UploadConflictKeepBoth:
		file, err = s.filesRepo.CreateFileAnyway(ctx, path, size, storageID)
	case UploadConflictSkip:
		file, skipped, err = s.filesRepo.CreateFileIfNotExists(ctx, path, size, storageID)
	default:
		return nil, false, domain.ErrBadRequest("invalid on_conflict value")
	}
	if err != nil {
		return nil, false, err
	}
	if skipped {
		return nil, true, nil
	}

	if err := s.manager.Upload(ctx, file, reader, progress); err != nil {
		return nil, false, err
	}

	return file, false, nil
}

func (s *FilesService) GetFileForDownload(ctx context.Context, userID, storageID uuid.UUID, path string) (*domain.File, error) {
	if err := requireStorageAccess(ctx, s.accessRepo, userID, storageID, domain.AccessRead); err != nil {
		return nil, err
	}

	return s.filesRepo.GetByPath(ctx, storageID, path)
}

func (s *FilesService) DownloadFileToWriter(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress) error {
	return s.manager.DownloadToWriter(ctx, file, w, progress)
}

func (s *FilesService) StreamFileToWriter(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress) error {
	return s.manager.StreamToWriter(ctx, file, w, progress)
}

func (s *FilesService) ExactFileSize(ctx context.Context, file *domain.File) (int64, error) {
	return s.manager.ExactFileSize(ctx, file)
}

func (s *FilesService) DownloadFileRangeToWriter(ctx context.Context, file *domain.File, w io.Writer, start, end, totalSize int64, progress *DownloadProgress) error {
	return s.manager.DownloadRangeToWriter(ctx, file, w, start, end, totalSize, progress)
}

func (s *FilesService) ListDir(ctx context.Context, userID, storageID uuid.UUID, path string) ([]domain.FSElement, error) {
	if err := requireStorageAccess(ctx, s.accessRepo, userID, storageID, domain.AccessRead); err != nil {
		return nil, err
	}

	return s.filesRepo.ListDir(ctx, storageID, path)
}

func (s *FilesService) Search(ctx context.Context, userID, storageID uuid.UUID, basePath, searchPath string) ([]domain.FSElement, error) {
	if err := requireStorageAccess(ctx, s.accessRepo, userID, storageID, domain.AccessRead); err != nil {
		return nil, err
	}

	return s.filesRepo.Search(ctx, storageID, basePath, searchPath)
}

func (s *FilesService) Delete(ctx context.Context, userID, storageID uuid.UUID, path string, progress *DeleteProgress, forceDelete bool) error {
	if err := requireStorageAccess(ctx, s.accessRepo, userID, storageID, domain.AccessAdmin); err != nil {
		return err
	}

	// Get chunks before deleting (CASCADE will remove them from DB)
	chunks, err := s.filesRepo.ListChunksByPath(ctx, storageID, path)
	if err != nil {
		return err
	}

	// Get storage info for Telegram deletion
	storage, err := s.storagesRepo.GetByID(ctx, storageID)
	if err != nil {
		return err
	}

	// Delete from Telegram first unless force delete was explicitly requested.
	// Force delete removes only DB records and may leave orphaned Telegram chunks.
	if !forceDelete && len(chunks) > 0 {
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
	if err := requireStorageAccess(ctx, s.accessRepo, userID, storageID, domain.AccessRead); err != nil {
		return "", err
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

	dirName := pathutil.ArchiveName(dirPath)

	zipWriter := zip.NewWriter(w)
	prefix := dirPath
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	if err := s.downloadDirFilesToZip(ctx, files, prefix, zipWriter, progress); err != nil {
		return "", err
	}

	if err := zipWriter.Close(); err != nil {
		return "", fmt.Errorf("closing zip archive: %w", err)
	}

	return dirName, nil
}

func (s *FilesService) downloadDirFilesToZip(ctx context.Context, files []domain.File, prefix string, zipWriter *zip.Writer, progress *DownloadProgress) error {
	for _, file := range files {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		relPath := strings.TrimPrefix(file.Path, prefix)
		entry, err := zipWriter.Create(relPath)
		if err != nil {
			return fmt.Errorf("creating zip entry for %s: %w", relPath, err)
		}

		if err := s.manager.DownloadToWriter(ctx, &file, entry, progress); err != nil {
			return fmt.Errorf("downloading %s into zip: %w", relPath, err)
		}
	}

	return nil
}

func (s *FilesService) Move(ctx context.Context, userID, storageID uuid.UUID, oldPath, newPath string) error {
	if err := requireStorageAccess(ctx, s.accessRepo, userID, storageID, domain.AccessWrite); err != nil {
		return err
	}

	return s.filesRepo.Move(ctx, storageID, oldPath, newPath)
}

func (s *FilesService) WorkersStatus(storageID uuid.UUID) string {
	if s.scheduler != nil && s.scheduler.IsWaiting(storageID) {
		return "waiting_rate_limit"
	}
	return "active"
}
