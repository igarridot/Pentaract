package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
)

type fakeFilesRepo struct {
	createFolderFn       func(ctx context.Context, storageID uuid.UUID, path string) error
	createFileAnywayFn   func(ctx context.Context, path string, size int64, storageID uuid.UUID) (*domain.File, error)
	getByPathFn          func(ctx context.Context, storageID uuid.UUID, path string) (*domain.File, error)
	listDirFn            func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.FSElement, error)
	searchFn             func(ctx context.Context, storageID uuid.UUID, basePath, searchPath string) ([]domain.FSElement, error)
	listChunksByPathFn   func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.FileChunk, error)
	deleteFn             func(ctx context.Context, storageID uuid.UUID, path string) error
	dirStatsFn           func(ctx context.Context, storageID uuid.UUID, path string) (int64, int64, error)
	listFilesUnderPathFn func(ctx context.Context, storageID uuid.UUID, path string) ([]domain.File, error)
	moveFn               func(ctx context.Context, storageID uuid.UUID, oldPath, newPath string) error
}

func (f *fakeFilesRepo) CreateFolder(ctx context.Context, storageID uuid.UUID, path string) error {
	return f.createFolderFn(ctx, storageID, path)
}
func (f *fakeFilesRepo) CreateFileAnyway(ctx context.Context, path string, size int64, storageID uuid.UUID) (*domain.File, error) {
	return f.createFileAnywayFn(ctx, path, size, storageID)
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
	f, err := svc.Upload(context.Background(), userID, storageID, "a.txt", 1, bytes.NewBufferString("x"), nil)
	if err != nil || f == nil {
		t.Fatalf("upload failed: %v", err)
	}
	if _, err := svc.GetFileForDownload(context.Background(), userID, storageID, "a.txt"); err != nil {
		t.Fatalf("get download file failed: %v", err)
	}
	if err := svc.DownloadFileToWriter(context.Background(), &domain.File{ID: fileID}, io.Discard, nil); err != nil {
		t.Fatalf("download writer failed: %v", err)
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

func TestFilesServiceForbiddenAndErrors(t *testing.T) {
	repo := &fakeFilesRepo{
		createFolderFn: func(ctx context.Context, storageID uuid.UUID, path string) error { return nil },
		createFileAnywayFn: func(ctx context.Context, path string, size int64, storageID uuid.UUID) (*domain.File, error) {
			return nil, errors.New("x")
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
	if _, err := svc.Upload(context.Background(), uuid.New(), uuid.New(), "a", 1, bytes.NewBufferString("x"), nil); err == nil {
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
