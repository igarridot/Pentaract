package service

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

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

type filesManagerDownloadWorkersLister interface {
	ListDownloadWorkers(ctx context.Context, storageID uuid.UUID) ([]repository.WorkerToken, error)
}

type filesManagerDownloadWithWorkers interface {
	DownloadToWriterWithWorkers(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress, workers []repository.WorkerToken) error
}

type dirArchiveWorkerPlan struct {
	streamWorkers  []repository.WorkerToken
	prefetchGroups [][]repository.WorkerToken
}

type dirArchiveFileJob struct {
	index   int
	file    domain.File
	relPath string
}

type dirArchiveFileResult struct {
	index    int
	relPath  string
	tempPath string
}

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

	if err := s.downloadDirFilesToZip(ctx, storageID, files, prefix, zipWriter, progress); err != nil {
		return "", err
	}

	if err := zipWriter.Close(); err != nil {
		return "", fmt.Errorf("closing zip archive: %w", err)
	}

	return dirName, nil
}

func buildDirArchiveWorkerPlan(workers []repository.WorkerToken, filesCount int) dirArchiveWorkerPlan {
	_ = filesCount

	plan := dirArchiveWorkerPlan{
		// Favor direct ZIP streaming over staging to temp files so the browser
		// can receive data earlier and we avoid extra local I/O per entry.
		streamWorkers: append([]repository.WorkerToken(nil), workers...),
	}
	return plan
}

func (s *FilesService) resolveDirArchiveWorkerPlan(ctx context.Context, storageID uuid.UUID, filesCount int) dirArchiveWorkerPlan {
	lister, ok := s.manager.(filesManagerDownloadWorkersLister)
	if !ok {
		return dirArchiveWorkerPlan{}
	}

	workers, err := lister.ListDownloadWorkers(ctx, storageID)
	if err != nil {
		return dirArchiveWorkerPlan{}
	}

	return buildDirArchiveWorkerPlan(workers, filesCount)
}

func (s *FilesService) downloadFileWithWorkers(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress, workers []repository.WorkerToken) error {
	if len(workers) > 0 {
		if downloader, ok := s.manager.(filesManagerDownloadWithWorkers); ok {
			return downloader.DownloadToWriterWithWorkers(ctx, file, w, progress, workers)
		}
	}

	return s.manager.DownloadToWriter(ctx, file, w, progress)
}

func (s *FilesService) downloadDirFilesToZip(ctx context.Context, storageID uuid.UUID, files []domain.File, prefix string, zipWriter *zip.Writer, progress *DownloadProgress) error {
	jobs := make([]dirArchiveFileJob, len(files))
	for index, file := range files {
		jobs[index] = dirArchiveFileJob{
			index:   index,
			file:    file,
			relPath: strings.TrimPrefix(file.Path, prefix),
		}
	}

	plan := s.resolveDirArchiveWorkerPlan(ctx, storageID, len(files))
	return s.downloadDirFilesToZipSequential(ctx, jobs, zipWriter, progress, plan.streamWorkers)
}

func (s *FilesService) downloadDirFilesToZipSequential(ctx context.Context, jobs []dirArchiveFileJob, zipWriter *zip.Writer, progress *DownloadProgress, workers []repository.WorkerToken) error {
	for _, job := range jobs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		entry, err := zipWriter.Create(job.relPath)
		if err != nil {
			return fmt.Errorf("creating zip entry for %s: %w", job.relPath, err)
		}

		if err := s.downloadFileWithWorkers(ctx, &job.file, entry, progress, workers); err != nil {
			return fmt.Errorf("downloading %s into zip: %w", job.relPath, err)
		}
	}

	return nil
}

func (s *FilesService) downloadDirFilesToZipPrefetch(
	ctx context.Context,
	jobs []dirArchiveFileJob,
	zipWriter *zip.Writer,
	progress *DownloadProgress,
	plan dirArchiveWorkerPlan,
) error {
	tempDir, err := os.MkdirTemp("", "pentaract-dir-download-*")
	if err != nil {
		return fmt.Errorf("creating temp dir for directory download: %w", err)
	}
	defer os.RemoveAll(tempDir)

	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	g, gctx := errgroup.WithContext(workCtx)
	jobCh := make(chan dirArchiveFileJob)
	resultBufferSize := len(jobs) - 1
	if resultBufferSize < 1 {
		resultBufferSize = 1
	}
	resultCh := make(chan dirArchiveFileResult, resultBufferSize)
	streamWorkersReady := make(chan struct{})

	startPrefetchWorker := func(workers []repository.WorkerToken, ready <-chan struct{}) {
		assignedWorkers := append([]repository.WorkerToken(nil), workers...)
		g.Go(func() error {
			if ready != nil {
				select {
				case <-ready:
				case <-gctx.Done():
					return gctx.Err()
				}
			}

			for {
				select {
				case <-gctx.Done():
					return gctx.Err()
				case job, ok := <-jobCh:
					if !ok {
						return nil
					}

					tempPath, err := s.downloadDirFileToTemp(gctx, tempDir, &job.file, progress, assignedWorkers)
					if err != nil {
						return fmt.Errorf("prefetching %s for zip: %w", job.relPath, err)
					}

					select {
					case resultCh <- dirArchiveFileResult{index: job.index, relPath: job.relPath, tempPath: tempPath}:
					case <-gctx.Done():
						_ = os.Remove(tempPath)
						return gctx.Err()
					}
				}
			}
		})
	}

	for _, workers := range plan.prefetchGroups {
		startPrefetchWorker(workers, nil)
	}
	startPrefetchWorker(plan.streamWorkers, streamWorkersReady)

	g.Go(func() error {
		defer close(jobCh)
		for _, job := range jobs[1:] {
			select {
			case jobCh <- job:
			case <-gctx.Done():
				return gctx.Err()
			}
		}
		return nil
	})

	errCh := make(chan error, 1)
	go func() {
		err := g.Wait()
		close(resultCh)
		errCh <- err
	}()

	firstEntry, err := zipWriter.Create(jobs[0].relPath)
	if err != nil {
		cancel()
		<-errCh
		return fmt.Errorf("creating zip entry for %s: %w", jobs[0].relPath, err)
	}
	if err := s.downloadFileWithWorkers(gctx, &jobs[0].file, firstEntry, progress, plan.streamWorkers); err != nil {
		cancel()
		<-errCh
		return fmt.Errorf("downloading %s into zip: %w", jobs[0].relPath, err)
	}
	close(streamWorkersReady)

	pending := make(map[int]dirArchiveFileResult, len(jobs)-1)
	nextIndex := 1
	var handleErr error

	for result := range resultCh {
		pending[result.index] = result

		for {
			next, ok := pending[nextIndex]
			if !ok {
				break
			}
			delete(pending, nextIndex)

			if handleErr == nil {
				if err := s.copyTempFileIntoZip(zipWriter, next.relPath, next.tempPath); err != nil {
					handleErr = err
					cancel()
				}
			} else {
				_ = os.Remove(next.tempPath)
			}
			nextIndex++
		}
	}

	workerErr := <-errCh
	if handleErr != nil {
		return handleErr
	}
	if workerErr != nil && !errors.Is(workerErr, context.Canceled) {
		return workerErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	return nil
}

func (s *FilesService) downloadDirFileToTemp(ctx context.Context, tempDir string, file *domain.File, progress *DownloadProgress, workers []repository.WorkerToken) (string, error) {
	tempFile, err := os.CreateTemp(tempDir, "dir-entry-*")
	if err != nil {
		return "", fmt.Errorf("creating temp file for %s: %w", file.Path, err)
	}

	tempPath := tempFile.Name()
	if err := s.downloadFileWithWorkers(ctx, file, tempFile, progress, workers); err != nil {
		tempFile.Close()
		_ = os.Remove(tempPath)
		return "", err
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("closing temp file for %s: %w", file.Path, err)
	}
	return tempPath, nil
}

func (s *FilesService) copyTempFileIntoZip(zipWriter *zip.Writer, relPath, tempPath string) error {
	entry, err := zipWriter.Create(relPath)
	if err != nil {
		_ = os.Remove(tempPath)
		return err
	}

	tempFile, err := os.Open(tempPath)
	if err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("opening temp file for %s: %w", relPath, err)
	}

	_, copyErr := io.Copy(entry, tempFile)
	closeErr := tempFile.Close()
	removeErr := os.Remove(tempPath)

	if copyErr != nil {
		return fmt.Errorf("copying %s into zip: %w", relPath, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("closing temp file for %s: %w", relPath, closeErr)
	}
	if removeErr != nil {
		return fmt.Errorf("removing temp file for %s: %w", relPath, removeErr)
	}
	return nil
}

func (s *FilesService) Move(ctx context.Context, userID, storageID uuid.UUID, oldPath, newPath string) error {
	if err := requireStorageAccess(ctx, s.accessRepo, userID, storageID, domain.AccessWrite); err != nil {
		return err
	}

	return s.filesRepo.Move(ctx, storageID, oldPath, newPath)
}

func (s *FilesService) EnsureWriteAccess(ctx context.Context, userID, storageID uuid.UUID) error {
	return requireStorageAccess(ctx, s.accessRepo, userID, storageID, domain.AccessWrite)
}

func (s *FilesService) WorkersStatus(storageID uuid.UUID) string {
	if s.scheduler != nil && s.scheduler.IsWaiting(storageID) {
		return "waiting_rate_limit"
	}
	return "active"
}
