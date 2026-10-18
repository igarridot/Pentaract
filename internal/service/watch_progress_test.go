package service

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
)

type fakeWatchProgressRepo struct {
	saved       []domain.WatchProgress
	savedFileID uuid.UUID
	listed      []string
	progress    map[string]domain.WatchProgress
	marked      []bool
	clearedUser uuid.UUID
	clearedDir  []string
	err         error
}

func (f *fakeWatchProgressRepo) DeleteAll(ctx context.Context, userID uuid.UUID) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.clearedUser = userID
	return 5, nil
}

func (f *fakeWatchProgressRepo) DeleteUnder(ctx context.Context, userID, storageID uuid.UUID, dirPrefix string) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.clearedUser = userID
	f.clearedDir = append(f.clearedDir, dirPrefix)
	return 2, nil
}

func (f *fakeWatchProgressRepo) SetWatched(ctx context.Context, userID, fileID uuid.UUID, watched bool) (*domain.WatchProgress, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.savedFileID = fileID
	f.marked = append(f.marked, watched)
	return &domain.WatchProgress{Watched: watched, DurationSeconds: 100}, nil
}

func (f *fakeWatchProgressRepo) Save(ctx context.Context, userID, fileID uuid.UUID, p domain.WatchProgress) error {
	if f.err != nil {
		return f.err
	}
	f.savedFileID = fileID
	f.saved = append(f.saved, p)
	return nil
}

func (f *fakeWatchProgressRepo) ListByPaths(ctx context.Context, userID, storageID uuid.UUID, paths []string) (map[string]domain.WatchProgress, error) {
	f.listed = paths
	return f.progress, f.err
}

func newTestWatchProgressService(repo *fakeWatchProgressRepo, hasAccess bool) (*WatchProgressService, uuid.UUID) {
	fileID := uuid.New()
	files := &fakeFilesRepo{
		getByPathFn: func(ctx context.Context, storageID uuid.UUID, path string) (*domain.File, error) {
			if path == "missing.mkv" {
				return nil, domain.ErrNotFound("file")
			}
			return &domain.File{ID: fileID, Path: path, StorageID: storageID}, nil
		},
	}
	access := &fakeFilesAccessRepo{
		hasAccessFn: func(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error) {
			return hasAccess && requiredLevel == domain.AccessRead, nil
		},
	}
	return NewWatchProgressService(repo, files, access), fileID
}

func TestWatchProgressAtAppliesResumeRules(t *testing.T) {
	cases := []struct {
		name               string
		position, duration float64
		wantPosition       float64
		wantWatched        bool
	}{
		{"middle keeps the position", 1800, 3600, 1800, false},
		{"barely started leaves nothing to resume", WatchResumeMinSeconds - 1, 3600, 0, false},
		{"exactly the minimum is kept", WatchResumeMinSeconds, 3600, WatchResumeMinSeconds, false},
		{"exactly 10% left is not watched yet", 3600 * WatchedFraction, 3600, 3600 * WatchedFraction, false},
		{"less than 10% left counts as watched", 3600*WatchedFraction + 1, 3600, 0, true},
		{"past the end is clamped and watched", 4000, 3600, 0, true},
		{"short clip near its end is watched", 19, 20, 0, true},
	}
	for _, tc := range cases {
		got := watchProgressAt(tc.position, tc.duration)
		if got.PositionSeconds != tc.wantPosition || got.Watched != tc.wantWatched || got.DurationSeconds != tc.duration {
			t.Errorf("%s: got %+v", tc.name, got)
		}
	}
}

func TestWatchProgressSaveStoresByFileID(t *testing.T) {
	repo := &fakeWatchProgressRepo{}
	svc, fileID := newTestWatchProgressService(repo, true)

	got, err := svc.Save(context.Background(), uuid.New(), uuid.New(), "movie.mkv", 600, 3600)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if repo.savedFileID != fileID || len(repo.saved) != 1 || repo.saved[0].PositionSeconds != 600 {
		t.Fatalf("unexpected save: file=%v saved=%+v", repo.savedFileID, repo.saved)
	}
	if got.PositionSeconds != 600 || got.UpdatedAt.IsZero() {
		t.Fatalf("expected the stored progress back, got %+v", got)
	}
}

func TestWatchProgressSaveRejects(t *testing.T) {
	ctx := context.Background()
	repo := &fakeWatchProgressRepo{}
	svc, _ := newTestWatchProgressService(repo, true)

	for _, bad := range [][2]float64{{-1, 100}, {10, 0}, {10, -5}, {math.NaN(), 100}, {10, math.Inf(1)}} {
		_, err := svc.Save(ctx, uuid.New(), uuid.New(), "movie.mkv", bad[0], bad[1])
		var appErr *domain.AppError
		if !errors.As(err, &appErr) || appErr.Code != 400 {
			t.Errorf("position=%v duration=%v: expected 400, got %v", bad[0], bad[1], err)
		}
	}

	if _, err := svc.Save(ctx, uuid.New(), uuid.New(), "missing.mkv", 10, 100); err == nil {
		t.Errorf("expected not found for a missing file")
	}

	denied, _ := newTestWatchProgressService(repo, false)
	if _, err := denied.Save(ctx, uuid.New(), uuid.New(), "movie.mkv", 10, 100); err == nil {
		t.Errorf("expected forbidden without access")
	}

	if len(repo.saved) != 0 {
		t.Fatalf("nothing should have been saved, got %+v", repo.saved)
	}

	failing, _ := newTestWatchProgressService(&fakeWatchProgressRepo{err: errors.New("db down")}, true)
	if _, err := failing.Save(ctx, uuid.New(), uuid.New(), "movie.mkv", 10, 100); err == nil {
		t.Errorf("expected the repository error")
	}
}

func TestWatchProgressAnnotateOnlyFiles(t *testing.T) {
	repo := &fakeWatchProgressRepo{progress: map[string]domain.WatchProgress{
		"a.mkv": {PositionSeconds: 42, DurationSeconds: 100},
		// A folder never gets progress, even if a path collided.
		"dir/": {PositionSeconds: 1, DurationSeconds: 2},
	}}
	svc, _ := newTestWatchProgressService(repo, true)
	elements := []domain.FSElement{
		{Path: "a.mkv", IsFile: true},
		{Path: "b.mkv", IsFile: true},
		{Path: "dir/", IsFile: false},
	}

	if err := svc.Annotate(context.Background(), uuid.New(), uuid.New(), elements); err != nil {
		t.Fatalf("annotate: %v", err)
	}
	if len(repo.listed) != 2 {
		t.Fatalf("only files should be looked up, got %v", repo.listed)
	}
	if elements[0].Watch == nil || elements[0].Watch.PositionSeconds != 42 || elements[1].Watch != nil || elements[2].Watch != nil {
		t.Fatalf("unexpected annotations: %+v", elements)
	}
}

func TestWatchProgressAnnotateSkipsQueryWithoutFiles(t *testing.T) {
	repo := &fakeWatchProgressRepo{err: errors.New("must not be called")}
	svc, _ := newTestWatchProgressService(repo, true)
	if err := svc.Annotate(context.Background(), uuid.New(), uuid.New(), []domain.FSElement{{Path: "dir/"}}); err != nil {
		t.Fatalf("annotate without files: %v", err)
	}

	elements := []domain.FSElement{{Path: "a.mkv", IsFile: true}}
	if err := svc.Annotate(context.Background(), uuid.New(), uuid.New(), elements); err == nil {
		t.Fatalf("expected the repository error")
	}
}

func TestWatchProgressSetWatchedMarksByFileID(t *testing.T) {
	ctx := context.Background()
	repo := &fakeWatchProgressRepo{}
	svc, fileID := newTestWatchProgressService(repo, true)

	for _, watched := range []bool{true, false} {
		got, err := svc.SetWatched(ctx, uuid.New(), uuid.New(), "movie.mkv", watched)
		if err != nil {
			t.Fatalf("set watched=%v: %v", watched, err)
		}
		if got.Watched != watched || repo.savedFileID != fileID {
			t.Fatalf("watched=%v: got %+v for file %v", watched, got, repo.savedFileID)
		}
	}
	if len(repo.marked) != 2 {
		t.Fatalf("expected two marks, got %v", repo.marked)
	}

	if _, err := svc.SetWatched(ctx, uuid.New(), uuid.New(), "missing.mkv", true); err == nil {
		t.Errorf("expected not found for a missing file")
	}
	denied, _ := newTestWatchProgressService(repo, false)
	if _, err := denied.SetWatched(ctx, uuid.New(), uuid.New(), "movie.mkv", true); err == nil {
		t.Errorf("expected forbidden without access")
	}
	if len(repo.marked) != 2 {
		t.Fatalf("a refused mark must not reach the repository, got %v", repo.marked)
	}
}

func TestWatchProgressClearAllIsPerUser(t *testing.T) {
	repo := &fakeWatchProgressRepo{}
	svc, _ := newTestWatchProgressService(repo, false)
	uid := uuid.New()

	n, err := svc.ClearAll(context.Background(), uid)
	if err != nil || n != 5 || repo.clearedUser != uid {
		t.Fatalf("unexpected clear: n=%d err=%v user=%v", n, err, repo.clearedUser)
	}

	failing, _ := newTestWatchProgressService(&fakeWatchProgressRepo{err: errors.New("db down")}, true)
	if _, err := failing.ClearAll(context.Background(), uid); err == nil {
		t.Fatalf("expected the repository error")
	}
}

func TestWatchProgressClearFolderMatchesTheFolderOnly(t *testing.T) {
	ctx := context.Background()
	repo := &fakeWatchProgressRepo{}
	svc, _ := newTestWatchProgressService(repo, true)
	uid := uuid.New()

	// The trailing slash keeps "series/b" from also clearing "series/bb".
	for _, path := range []string{"series/b", "series/b/", "series/b//"} {
		n, err := svc.ClearFolder(ctx, uid, uuid.New(), path)
		if err != nil || n != 2 {
			t.Fatalf("%q: n=%d err=%v", path, n, err)
		}
	}
	for _, dir := range repo.clearedDir {
		if dir != "series/b/" {
			t.Fatalf("expected the folder prefix, got %v", repo.clearedDir)
		}
	}
	if repo.clearedUser != uid {
		t.Fatalf("expected the caller's progress only, got user %v", repo.clearedUser)
	}
}

func TestWatchProgressClearFolderRejects(t *testing.T) {
	ctx := context.Background()
	repo := &fakeWatchProgressRepo{}
	svc, _ := newTestWatchProgressService(repo, true)

	// An empty path would be every folder of the storage.
	_, err := svc.ClearFolder(ctx, uuid.New(), uuid.New(), "")
	var appErr *domain.AppError
	if !errors.As(err, &appErr) || appErr.Code != 400 {
		t.Fatalf("expected 400 for an empty path, got %v", err)
	}

	denied, _ := newTestWatchProgressService(repo, false)
	if _, err := denied.ClearFolder(ctx, uuid.New(), uuid.New(), "series"); err == nil {
		t.Fatalf("expected forbidden without access")
	}
	if len(repo.clearedDir) != 0 {
		t.Fatalf("a refused clear must not reach the repository, got %v", repo.clearedDir)
	}
}
