package service

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
)

type fakeFilesRepo struct {
	createFolderFn          func(ctx context.Context, storageID uuid.UUID, path string) error
	createFileAnywayFn      func(ctx context.Context, path string, size int64, storageID uuid.UUID) (*domain.File, error)
	createFileIfNotExistsFn func(ctx context.Context, path string, size int64, storageID uuid.UUID) (*domain.File, bool, error)
	getByPathFn             func(ctx context.Context, storageID uuid.UUID, path string) (*domain.File, error)
	listDirFn               func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.FSElement, error)
	searchFn                func(ctx context.Context, storageID uuid.UUID, basePath, searchPath string) ([]domain.FSElement, error)
	listChunksByPathFn      func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.FileChunk, error)
	deleteFn                func(ctx context.Context, storageID uuid.UUID, path string) error
	dirStatsFn              func(ctx context.Context, storageID uuid.UUID, path string) (int64, int64, error)
	listFilesUnderPathFn    func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.File, error)
	moveFn                  func(ctx context.Context, storageID uuid.UUID, oldPath, newPath string) error
}

func (f *fakeFilesRepo) CreateFolder(ctx context.Context, storageID uuid.UUID, path string) error {
	return f.createFolderFn(ctx, storageID, path)
}
func (f *fakeFilesRepo) CreateFileAnyway(ctx context.Context, path string, size int64, storageID uuid.UUID) (*domain.File, error) {
	return f.createFileAnywayFn(ctx, path, size, storageID)
}
func (f *fakeFilesRepo) CreateFileIfNotExists(ctx context.Context, path string, size int64, storageID uuid.UUID) (*domain.File, bool, error) {
	return f.createFileIfNotExistsFn(ctx, path, size, storageID)
}
func (f *fakeFilesRepo) GetByPath(ctx context.Context, storageID uuid.UUID, path string) (*domain.File, error) {
	return f.getByPathFn(ctx, storageID, path)
}
func (f *fakeFilesRepo) ListDir(ctx context.Context, storageID uuid.UUID, path string) ([]domain.FSElement, error) {
	return f.listDirFn(ctx, storageID, path)
}
func (f *fakeFilesRepo) Search(ctx context.Context, storageID uuid.UUID, basePath, searchPath string) ([]domain.FSElement, error) {
	return f.searchFn(ctx, storageID, basePath, searchPath)
}
func (f *fakeFilesRepo) ListChunksByPath(ctx context.Context, storageID uuid.UUID, path string) ([]domain.FileChunk, error) {
	return f.listChunksByPathFn(ctx, storageID, path)
}
func (f *fakeFilesRepo) Delete(ctx context.Context, storageID uuid.UUID, path string) error {
	return f.deleteFn(ctx, storageID, path)
}
func (f *fakeFilesRepo) DirStats(ctx context.Context, storageID uuid.UUID, path string) (int64, int64, error) {
	return f.dirStatsFn(ctx, storageID, path)
}
func (f *fakeFilesRepo) ListFilesUnderPath(ctx context.Context, storageID uuid.UUID, path string) ([]domain.File, error) {
	return f.listFilesUnderPathFn(ctx, storageID, path)
}
func (f *fakeFilesRepo) Move(ctx context.Context, storageID uuid.UUID, oldPath, newPath string) error {
	return f.moveFn(ctx, storageID, oldPath, newPath)
}

type fakeFilesAccessRepo struct {
	hasAccessFn func(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error)
}

func (f *fakeFilesAccessRepo) HasAccess(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error) {
	return f.hasAccessFn(ctx, userID, storageID, requiredLevel)
}

type fakeFilesManager struct {
	uploadFn             func(ctx context.Context, file *domain.File, reader io.Reader, progress *UploadProgress) error
	downloadFn           func(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress) error
	streamFn             func(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress) error
	exactFn              func(ctx context.Context, file *domain.File) (int64, error)
	rangeFn              func(ctx context.Context, file *domain.File, w io.Writer, start, end, totalSize int64, progress *DownloadProgress) error
	deleteFromTelegramFn func(ctx context.Context, storage domain.Storage, chunks []domain.FileChunk, progress *DeleteProgress) error
}

func (f *fakeFilesManager) Upload(ctx context.Context, file *domain.File, reader io.Reader, progress *UploadProgress) error {
	return f.uploadFn(ctx, file, reader, progress)
}
func (f *fakeFilesManager) DownloadToWriter(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress) error {
	return f.downloadFn(ctx, file, w, progress)
}
func (f *fakeFilesManager) StreamToWriter(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress) error {
	if f.streamFn == nil {
		return f.downloadFn(ctx, file, w, progress)
	}
	return f.streamFn(ctx, file, w, progress)
}
func (f *fakeFilesManager) ExactFileSize(ctx context.Context, file *domain.File) (int64, error) {
	return f.exactFn(ctx, file)
}
func (f *fakeFilesManager) DownloadRangeToWriter(ctx context.Context, file *domain.File, w io.Writer, start, end, totalSize int64, progress *DownloadProgress) error {
	return f.rangeFn(ctx, file, w, start, end, totalSize, progress)
}
func (f *fakeFilesManager) DeleteFromTelegram(ctx context.Context, storage domain.Storage, chunks []domain.FileChunk, progress *DeleteProgress) error {
	return f.deleteFromTelegramFn(ctx, storage, chunks, progress)
}

type fakeStorageRepo struct {
	getByIDFn func(ctx context.Context, id uuid.UUID) (*domain.Storage, error)
}

func (f *fakeStorageRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Storage, error) {
	return f.getByIDFn(ctx, id)
}

func TestFilesServiceMainFlows(t *testing.T) {
	userID := uuid.New()
	storageID := uuid.New()
	fileID := uuid.New()
	repo := &fakeFilesRepo{
		createFolderFn: func(ctx context.Context, storageID uuid.UUID, path string) error { return nil },
		createFileAnywayFn: func(ctx context.Context, path string, size int64, storageID uuid.UUID) (*domain.File, error) {
			return &domain.File{ID: fileID, Path: path, StorageID: storageID, Size: size}, nil
		},
		createFileIfNotExistsFn: func(ctx context.Context, path string, size int64, storageID uuid.UUID) (*domain.File, bool, error) {
			return &domain.File{ID: fileID, Path: path, StorageID: storageID, Size: size}, false, nil
		},
		getByPathFn: func(ctx context.Context, storageID uuid.UUID, path string) (*domain.File, error) {
			return &domain.File{ID: fileID, Path: path, StorageID: storageID}, nil
		},
		listDirFn: func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.FSElement, error) {
			return []domain.FSElement{{Name: "x"}}, nil
		},
		searchFn: func(ctx context.Context, storageID uuid.UUID, basePath, searchPath string) ([]domain.FSElement, error) {
			return []domain.FSElement{{Name: "x"}}, nil
		},
		listChunksByPathFn: func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.FileChunk, error) {
			return []domain.FileChunk{{TelegramMessageID: 1}}, nil
		},
		deleteFn:   func(ctx context.Context, storageID uuid.UUID, path string) error { return nil },
		dirStatsFn: func(ctx context.Context, storageID uuid.UUID, path string) (int64, int64, error) { return 10, 1, nil },
		listFilesUnderPathFn: func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.File, error) {
			return []domain.File{{ID: fileID, Path: "dir/a.txt", StorageID: storageID}}, nil
		},
		moveFn: func(ctx context.Context, storageID uuid.UUID, oldPath, newPath string) error { return nil },
	}
	access := &fakeFilesAccessRepo{
		hasAccessFn: func(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error) {
			return true, nil
		},
	}
	manager := &fakeFilesManager{
		uploadFn: func(ctx context.Context, file *domain.File, reader io.Reader, progress *UploadProgress) error {
			return nil
		},
		downloadFn: func(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress) error {
			_, _ = w.Write([]byte("ok"))
			return nil
		},
		streamFn: func(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress) error {
			_, _ = w.Write([]byte("stream"))
			return nil
		},
		exactFn: func(ctx context.Context, file *domain.File) (int64, error) { return 2, nil },
		rangeFn: func(ctx context.Context, file *domain.File, w io.Writer, start, end, totalSize int64, progress *DownloadProgress) error {
			_, _ = w.Write([]byte("ok"))
			return nil
		},
		deleteFromTelegramFn: func(ctx context.Context, storage domain.Storage, chunks []domain.FileChunk, progress *DeleteProgress) error {
			return nil
		},
	}
	storages := &fakeStorageRepo{getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Storage, error) {
		return &domain.Storage{ID: id, Name: "S"}, nil
	}}
	svc := NewFilesServiceWithDeps(repo, access, manager, storages, nil)

	if err := svc.CreateFolder(context.Background(), userID, storageID, "base", "folder"); err != nil {
		t.Fatalf("create folder failed: %v", err)
	}
	f, skipped, err := svc.Upload(context.Background(), userID, storageID, "a.txt", 1, bytes.NewBufferString("x"), nil, UploadConflictKeepBoth)
	if err != nil || f == nil || skipped {
		t.Fatalf("upload failed: %v", err)
	}
	if _, err := svc.GetFileForDownload(context.Background(), userID, storageID, "a.txt"); err != nil {
		t.Fatalf("get download file failed: %v", err)
	}
	if err := svc.DownloadFileToWriter(context.Background(), &domain.File{ID: fileID}, io.Discard, nil); err != nil {
		t.Fatalf("download writer failed: %v", err)
	}
	if err := svc.StreamFileToWriter(context.Background(), &domain.File{ID: fileID}, io.Discard, nil); err != nil {
		t.Fatalf("stream writer failed: %v", err)
	}
	if _, err := svc.ExactFileSize(context.Background(), &domain.File{ID: fileID}); err != nil {
		t.Fatalf("exact size failed: %v", err)
	}
	if err := svc.DownloadFileRangeToWriter(context.Background(), &domain.File{ID: fileID}, io.Discard, 0, 1, 2, nil); err != nil {
		t.Fatalf("range failed: %v", err)
	}
	if _, err := svc.ListDir(context.Background(), userID, storageID, ""); err != nil {
		t.Fatalf("list dir failed: %v", err)
	}
	if _, err := svc.Search(context.Background(), userID, storageID, "", "a"); err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if err := svc.Delete(context.Background(), userID, storageID, "a.txt", nil, false); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if _, err := svc.DownloadDir(context.Background(), userID, storageID, "dir", io.Discard, &DownloadProgress{}); err != nil {
		t.Fatalf("download dir failed: %v", err)
	}
	if err := svc.Move(context.Background(), userID, storageID, "a.txt", "b.txt"); err != nil {
		t.Fatalf("move failed: %v", err)
	}
}

func TestFilesServiceCreateFolderNormalizesPath(t *testing.T) {
	userID := uuid.New()
	storageID := uuid.New()
	var gotPath string

	repo := &fakeFilesRepo{
		createFolderFn: func(ctx context.Context, storageID uuid.UUID, path string) error {
			gotPath = path
			return nil
		},
	}
	access := &fakeFilesAccessRepo{
		hasAccessFn: func(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error) {
			return true, nil
		},
	}
	svc := NewFilesServiceWithDeps(repo, access, &fakeFilesManager{}, &fakeStorageRepo{}, nil)

	if err := svc.CreateFolder(context.Background(), userID, storageID, "/base/sub/", "/child/"); err != nil {
		t.Fatalf("create folder failed: %v", err)
	}
	if gotPath != "base/sub/child" {
		t.Fatalf("expected normalized path base/sub/child, got %q", gotPath)
	}
}

func TestFilesServiceCreateFolderRejectsInvalidName(t *testing.T) {
	repo := &fakeFilesRepo{
		createFolderFn: func(ctx context.Context, storageID uuid.UUID, path string) error {
			return nil
		},
	}
	access := &fakeFilesAccessRepo{
		hasAccessFn: func(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error) {
			return true, nil
		},
	}
	svc := NewFilesServiceWithDeps(repo, access, &fakeFilesManager{}, &fakeStorageRepo{}, nil)

	if err := svc.CreateFolder(context.Background(), uuid.New(), uuid.New(), "base", "a/b"); err == nil {
		t.Fatalf("expected bad request for folder name with slash")
	}
	if err := svc.CreateFolder(context.Background(), uuid.New(), uuid.New(), "base", "///"); err == nil {
		t.Fatalf("expected bad request for empty folder name after trimming")
	}
}

func TestFilesServiceForbiddenAndErrors(t *testing.T) {
	repo := &fakeFilesRepo{
		createFolderFn: func(ctx context.Context, storageID uuid.UUID, path string) error { return nil },
		createFileAnywayFn: func(ctx context.Context, path string, size int64, storageID uuid.UUID) (*domain.File, error) {
			return nil, errors.New("x")
		},
		createFileIfNotExistsFn: func(ctx context.Context, path string, size int64, storageID uuid.UUID) (*domain.File, bool, error) {
			return nil, false, errors.New("x")
		},
		getByPathFn: func(ctx context.Context, storageID uuid.UUID, path string) (*domain.File, error) {
			return nil, errors.New("x")
		},
		listDirFn: func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.FSElement, error) {
			return nil, errors.New("x")
		},
		searchFn: func(ctx context.Context, storageID uuid.UUID, basePath, searchPath string) ([]domain.FSElement, error) {
			return nil, errors.New("x")
		},
		listChunksByPathFn: func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.FileChunk, error) {
			return nil, errors.New("x")
		},
		deleteFn:             func(ctx context.Context, storageID uuid.UUID, path string) error { return nil },
		dirStatsFn:           func(ctx context.Context, storageID uuid.UUID, path string) (int64, int64, error) { return 0, 0, nil },
		listFilesUnderPathFn: func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.File, error) { return nil, nil },
		moveFn:               func(ctx context.Context, storageID uuid.UUID, oldPath, newPath string) error { return nil },
	}
	access := &fakeFilesAccessRepo{
		hasAccessFn: func(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error) {
			return false, nil
		},
	}
	manager := &fakeFilesManager{
		uploadFn: func(ctx context.Context, file *domain.File, reader io.Reader, progress *UploadProgress) error {
			return nil
		},
		downloadFn: func(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress) error {
			return nil
		},
		exactFn: func(ctx context.Context, file *domain.File) (int64, error) { return 0, nil },
		rangeFn: func(ctx context.Context, file *domain.File, w io.Writer, start, end, totalSize int64, progress *DownloadProgress) error {
			return nil
		},
		deleteFromTelegramFn: func(ctx context.Context, storage domain.Storage, chunks []domain.FileChunk, progress *DeleteProgress) error {
			return nil
		},
	}
	storages := &fakeStorageRepo{getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Storage, error) { return nil, errors.New("x") }}
	svc := NewFilesServiceWithDeps(repo, access, manager, storages, nil)

	if err := svc.CreateFolder(context.Background(), uuid.New(), uuid.New(), "", "folder"); err == nil {
		t.Fatalf("expected forbidden")
	}
	if _, _, err := svc.Upload(context.Background(), uuid.New(), uuid.New(), "a", 1, bytes.NewBufferString("x"), nil, UploadConflictKeepBoth); err == nil {
		t.Fatalf("expected forbidden")
	}
	if _, err := svc.GetFileForDownload(context.Background(), uuid.New(), uuid.New(), "a"); err == nil {
		t.Fatalf("expected forbidden")
	}
	if _, err := svc.ListDir(context.Background(), uuid.New(), uuid.New(), ""); err == nil {
		t.Fatalf("expected forbidden")
	}
	if _, err := svc.Search(context.Background(), uuid.New(), uuid.New(), "", "a"); err == nil {
		t.Fatalf("expected forbidden")
	}
	if err := svc.Delete(context.Background(), uuid.New(), uuid.New(), "a", nil, false); err == nil {
		t.Fatalf("expected forbidden")
	}
	if err := svc.Move(context.Background(), uuid.New(), uuid.New(), "a", "b"); err == nil {
		t.Fatalf("expected forbidden")
	}
}

func TestFilesServiceWorkersStatus(t *testing.T) {
	svc := NewFilesServiceWithDeps(nil, nil, nil, nil, nil)
	if got := svc.WorkersStatus(uuid.New()); got != "active" {
		t.Fatalf("expected active with nil scheduler, got %q", got)
	}

	storageID := uuid.New()
	scheduler := NewWorkerSchedulerWithRepo(nil, 1)
	scheduler.waiting[storageID] = 1
	svc = NewFilesServiceWithDeps(nil, nil, nil, nil, scheduler)
	if got := svc.WorkersStatus(storageID); got != "waiting_rate_limit" {
		t.Fatalf("expected waiting_rate_limit, got %q", got)
	}
}

func TestFilesServiceUploadSkipConflict(t *testing.T) {
	userID := uuid.New()
	storageID := uuid.New()
	access := &fakeFilesAccessRepo{
		hasAccessFn: func(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error) {
			return true, nil
		},
	}
	managerCalled := false
	repo := &fakeFilesRepo{
		createFolderFn: func(ctx context.Context, storageID uuid.UUID, path string) error { return nil },
		createFileAnywayFn: func(ctx context.Context, path string, size int64, storageID uuid.UUID) (*domain.File, error) {
			return &domain.File{}, nil
		},
		createFileIfNotExistsFn: func(ctx context.Context, path string, size int64, storageID uuid.UUID) (*domain.File, bool, error) {
			return nil, true, nil
		},
		getByPathFn: func(ctx context.Context, storageID uuid.UUID, path string) (*domain.File, error) { return nil, nil },
		listDirFn: func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.FSElement, error) {
			return nil, nil
		},
		searchFn: func(ctx context.Context, storageID uuid.UUID, basePath, searchPath string) ([]domain.FSElement, error) {
			return nil, nil
		},
		listChunksByPathFn: func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.FileChunk, error) {
			return nil, nil
		},
		deleteFn:             func(ctx context.Context, storageID uuid.UUID, path string) error { return nil },
		dirStatsFn:           func(ctx context.Context, storageID uuid.UUID, path string) (int64, int64, error) { return 0, 0, nil },
		listFilesUnderPathFn: func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.File, error) { return nil, nil },
		moveFn:               func(ctx context.Context, storageID uuid.UUID, oldPath, newPath string) error { return nil },
	}
	manager := &fakeFilesManager{
		uploadFn: func(ctx context.Context, file *domain.File, reader io.Reader, progress *UploadProgress) error {
			managerCalled = true
			return nil
		},
		downloadFn: func(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress) error {
			return nil
		},
		exactFn: func(ctx context.Context, file *domain.File) (int64, error) { return 0, nil },
		rangeFn: func(ctx context.Context, file *domain.File, w io.Writer, start, end, totalSize int64, progress *DownloadProgress) error {
			return nil
		},
		deleteFromTelegramFn: func(ctx context.Context, storage domain.Storage, chunks []domain.FileChunk, progress *DeleteProgress) error {
			return nil
		},
	}
	svc := NewFilesServiceWithDeps(repo, access, manager, &fakeStorageRepo{getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Storage, error) {
		return &domain.Storage{ID: id}, nil
	}}, nil)

	file, skipped, err := svc.Upload(context.Background(), userID, storageID, "a.txt", 1, bytes.NewBufferString("x"), nil, UploadConflictSkip)
	if err != nil {
		t.Fatalf("upload skip returned error: %v", err)
	}
	if file != nil || !skipped {
		t.Fatalf("expected skipped upload, got file=%v skipped=%v", file, skipped)
	}
	if managerCalled {
		t.Fatalf("manager upload should not be called when skipped")
	}
}

func TestFilesServiceUploadInvalidConflictPolicy(t *testing.T) {
	access := &fakeFilesAccessRepo{
		hasAccessFn: func(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error) {
			return true, nil
		},
	}
	repo := &fakeFilesRepo{
		createFolderFn: func(ctx context.Context, storageID uuid.UUID, path string) error { return nil },
		createFileAnywayFn: func(ctx context.Context, path string, size int64, storageID uuid.UUID) (*domain.File, error) {
			return &domain.File{}, nil
		},
		createFileIfNotExistsFn: func(ctx context.Context, path string, size int64, storageID uuid.UUID) (*domain.File, bool, error) {
			return &domain.File{}, false, nil
		},
		getByPathFn: func(ctx context.Context, storageID uuid.UUID, path string) (*domain.File, error) { return nil, nil },
		listDirFn: func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.FSElement, error) {
			return nil, nil
		},
		searchFn: func(ctx context.Context, storageID uuid.UUID, basePath, searchPath string) ([]domain.FSElement, error) {
			return nil, nil
		},
		listChunksByPathFn: func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.FileChunk, error) {
			return nil, nil
		},
		deleteFn:             func(ctx context.Context, storageID uuid.UUID, path string) error { return nil },
		dirStatsFn:           func(ctx context.Context, storageID uuid.UUID, path string) (int64, int64, error) { return 0, 0, nil },
		listFilesUnderPathFn: func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.File, error) { return nil, nil },
		moveFn:               func(ctx context.Context, storageID uuid.UUID, oldPath, newPath string) error { return nil },
	}
	manager := &fakeFilesManager{
		uploadFn: func(ctx context.Context, file *domain.File, reader io.Reader, progress *UploadProgress) error {
			return nil
		},
		downloadFn: func(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress) error {
			return nil
		},
		exactFn: func(ctx context.Context, file *domain.File) (int64, error) { return 0, nil },
		rangeFn: func(ctx context.Context, file *domain.File, w io.Writer, start, end, totalSize int64, progress *DownloadProgress) error {
			return nil
		},
		deleteFromTelegramFn: func(ctx context.Context, storage domain.Storage, chunks []domain.FileChunk, progress *DeleteProgress) error {
			return nil
		},
	}
	svc := NewFilesServiceWithDeps(repo, access, manager, &fakeStorageRepo{getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Storage, error) {
		return &domain.Storage{ID: id}, nil
	}}, nil)

	if _, _, err := svc.Upload(context.Background(), uuid.New(), uuid.New(), "a.txt", 1, bytes.NewBufferString("x"), nil, "invalid"); err == nil {
		t.Fatalf("expected invalid on_conflict error")
	}
}

func TestFilesServiceDeleteSkipsTelegramCleanupWhenNotNeeded(t *testing.T) {
	access := &fakeFilesAccessRepo{
		hasAccessFn: func(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error) {
			return true, nil
		},
	}
	storageRepo := &fakeStorageRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.Storage, error) {
			return &domain.Storage{ID: id, Name: "Docs"}, nil
		},
	}

	tests := []struct {
		name        string
		forceDelete bool
		chunks      []domain.FileChunk
	}{
		{name: "force delete bypasses telegram cleanup", forceDelete: true, chunks: []domain.FileChunk{{TelegramMessageID: 1}}},
		{name: "empty chunk list bypasses telegram cleanup", forceDelete: false, chunks: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deleteCalled := false
			managerCalled := false
			repo := &fakeFilesRepo{
				listChunksByPathFn: func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.FileChunk, error) {
					return tt.chunks, nil
				},
				deleteFn: func(ctx context.Context, storageID uuid.UUID, path string) error {
					deleteCalled = true
					return nil
				},
			}
			manager := &fakeFilesManager{
				deleteFromTelegramFn: func(ctx context.Context, storage domain.Storage, chunks []domain.FileChunk, progress *DeleteProgress) error {
					managerCalled = true
					return nil
				},
			}

			svc := NewFilesServiceWithDeps(repo, access, manager, storageRepo, nil)
			if err := svc.Delete(context.Background(), uuid.New(), uuid.New(), "docs/report.pdf", nil, tt.forceDelete); err != nil {
				t.Fatalf("Delete returned error: %v", err)
			}
			if managerCalled {
				t.Fatalf("DeleteFromTelegram should not be called")
			}
			if !deleteCalled {
				t.Fatalf("expected repository delete to run")
			}
		})
	}
}

func TestFilesServiceDownloadDirReturnsArchiveName(t *testing.T) {
	access := &fakeFilesAccessRepo{
		hasAccessFn: func(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error) {
			return true, nil
		},
	}
	repo := &fakeFilesRepo{
		listFilesUnderPathFn: func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.File, error) {
			if path == "" {
				return []domain.File{{ID: uuid.New(), Path: "root.txt", StorageID: storageID}}, nil
			}
			return []domain.File{{ID: uuid.New(), Path: "docs/report.txt", StorageID: storageID}}, nil
		},
	}
	manager := &fakeFilesManager{
		downloadFn: func(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress) error {
			_, _ = io.WriteString(w, "payload")
			return nil
		},
	}
	svc := NewFilesServiceWithDeps(repo, access, manager, &fakeStorageRepo{}, nil)

	if name, err := svc.DownloadDir(context.Background(), uuid.New(), uuid.New(), "", io.Discard, nil); err != nil || name != "files" {
		t.Fatalf("root archive name = %q err=%v, want files", name, err)
	}
	if name, err := svc.DownloadDir(context.Background(), uuid.New(), uuid.New(), "root/docs/", io.Discard, nil); err != nil || name != "docs" {
		t.Fatalf("nested archive name = %q err=%v, want docs", name, err)
	}
}

func TestFilesServiceDownloadDirWritesValidZipArchive(t *testing.T) {
	userID := uuid.New()
	storageID := uuid.New()
	fileOneID := uuid.New()
	fileTwoID := uuid.New()

	access := &fakeFilesAccessRepo{
		hasAccessFn: func(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error) {
			return true, nil
		},
	}
	repo := &fakeFilesRepo{
		dirStatsFn: func(ctx context.Context, storageID uuid.UUID, path string) (int64, int64, error) {
			return int64(len("hello") + len("world")), 2, nil
		},
		listFilesUnderPathFn: func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.File, error) {
			return []domain.File{
				{ID: fileOneID, Path: "root/docs/report.txt", StorageID: storageID},
				{ID: fileTwoID, Path: "root/docs/sub/image.txt", StorageID: storageID},
			}, nil
		},
	}
	manager := &fakeFilesManager{
		downloadFn: func(ctx context.Context, file *domain.File, w io.Writer, progress *DownloadProgress) error {
			switch file.ID {
			case fileOneID:
				_, _ = io.WriteString(w, "hello")
			case fileTwoID:
				_, _ = io.WriteString(w, "world")
			default:
				t.Fatalf("unexpected file id %s", file.ID)
			}
			return nil
		},
	}
	svc := NewFilesServiceWithDeps(repo, access, manager, &fakeStorageRepo{}, nil)

	var archive bytes.Buffer
	progress := &DownloadProgress{}
	name, err := svc.DownloadDir(context.Background(), userID, storageID, "root/docs", &archive, progress)
	if err != nil {
		t.Fatalf("download dir failed: %v", err)
	}
	if name != "docs" {
		t.Fatalf("archive name = %q, want docs", name)
	}
	if progress.TotalChunks != 2 || progress.TotalBytes != int64(len("hello")+len("world")) {
		t.Fatalf("unexpected totals: chunks=%d bytes=%d", progress.TotalChunks, progress.TotalBytes)
	}

	reader, err := zip.NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		t.Fatalf("zip reader failed: %v", err)
	}
	if len(reader.File) != 2 {
		t.Fatalf("zip entry count = %d, want 2", len(reader.File))
	}

	entries := map[string]string{}
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", file.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read zip entry %s: %v", file.Name, err)
		}
		entries[file.Name] = string(content)
	}

	if entries["report.txt"] != "hello" {
		t.Fatalf("unexpected report.txt contents: %q", entries["report.txt"])
	}
	if entries["sub/image.txt"] != "world" {
		t.Fatalf("unexpected sub/image.txt contents: %q", entries["sub/image.txt"])
	}
}
