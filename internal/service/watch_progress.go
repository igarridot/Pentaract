package service

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/pathutil"
)

// WatchProgressService records where each user is in each video, so the web
// UI and Kodi resume from the same point and show the same watched marks.
type WatchProgressService struct {
	repo       watchProgressRepository
	filesRepo  watchFilesRepository
	accessRepo filesAccessRepository
}

type watchProgressRepository interface {
	Save(ctx context.Context, userID, fileID uuid.UUID, p domain.WatchProgress) error
	SetWatched(ctx context.Context, userID, fileID uuid.UUID, watched bool) (*domain.WatchProgress, error)
	ListByPaths(ctx context.Context, userID, storageID uuid.UUID, paths []string) (map[string]domain.WatchProgress, error)
	DeleteAll(ctx context.Context, userID uuid.UUID) (int64, error)
	DeleteUnder(ctx context.Context, userID, storageID uuid.UUID, dirPrefix string) (int64, error)
}

type watchFilesRepository interface {
	GetByPath(ctx context.Context, storageID uuid.UUID, path string) (*domain.File, error)
}

func NewWatchProgressService(repo watchProgressRepository, filesRepo watchFilesRepository, accessRepo filesAccessRepository) *WatchProgressService {
	return &WatchProgressService{repo: repo, filesRepo: filesRepo, accessRepo: accessRepo}
}

// Save records that the user is at position (seconds) of a video lasting
// duration seconds, and returns what was stored.
func (s *WatchProgressService) Save(ctx context.Context, userID, storageID uuid.UUID, path string, position, duration float64) (*domain.WatchProgress, error) {
	if !validSeconds(duration) || duration == 0 || !validSeconds(position) {
		return nil, domain.ErrBadRequest("position and duration must be seconds, and duration above zero")
	}
	file, err := s.readableFile(ctx, userID, storageID, path)
	if err != nil {
		return nil, err
	}

	p := watchProgressAt(position, duration)
	p.UpdatedAt = time.Now()
	if err := s.repo.Save(ctx, userID, file.ID, p); err != nil {
		return nil, err
	}
	return &p, nil
}

// SetWatched marks a file as watched or pending by hand (web UI or Kodi
// menu). Either way there is nothing left to resume.
func (s *WatchProgressService) SetWatched(ctx context.Context, userID, storageID uuid.UUID, path string, watched bool) (*domain.WatchProgress, error) {
	file, err := s.readableFile(ctx, userID, storageID, path)
	if err != nil {
		return nil, err
	}
	return s.repo.SetWatched(ctx, userID, file.ID, watched)
}

// ClearAll forgets everything the user has played, in every storage, and
// returns how many files it was about. Only the user's own marks go.
func (s *WatchProgressService) ClearAll(ctx context.Context, userID uuid.UUID) (int64, error) {
	return s.repo.DeleteAll(ctx, userID)
}

// ClearFolder forgets what the user has played inside a folder and all its
// subfolders, and returns how many files it was about.
func (s *WatchProgressService) ClearFolder(ctx context.Context, userID, storageID uuid.UUID, path string) (int64, error) {
	dir := pathutil.DirPrefix(path)
	if dir == "" {
		return 0, domain.ErrBadRequest("folder path is required")
	}
	if err := requireStorageAccess(ctx, s.accessRepo, userID, storageID, domain.AccessRead); err != nil {
		return 0, err
	}
	return s.repo.DeleteUnder(ctx, userID, storageID, dir)
}

func (s *WatchProgressService) readableFile(ctx context.Context, userID, storageID uuid.UUID, path string) (*domain.File, error) {
	if err := requireStorageAccess(ctx, s.accessRepo, userID, storageID, domain.AccessRead); err != nil {
		return nil, err
	}
	return s.filesRepo.GetByPath(ctx, storageID, path)
}

// Annotate sets Watch on the files among elements the user has played.
// The caller has already checked the user may list them.
func (s *WatchProgressService) Annotate(ctx context.Context, userID, storageID uuid.UUID, elements []domain.FSElement) error {
	var paths []string
	for _, el := range elements {
		if el.IsFile {
			paths = append(paths, el.Path)
		}
	}
	if len(paths) == 0 {
		return nil
	}

	progress, err := s.repo.ListByPaths(ctx, userID, storageID, paths)
	if err != nil {
		return err
	}
	for i := range elements {
		if p, ok := progress[elements[i].Path]; ok && elements[i].IsFile {
			elements[i].Watch = &p
		}
	}
	return nil
}

// watchProgressAt applies the resume rules to a position a player reported:
// with less than 10% left it counts as watched and leaves nothing to resume
// (credits); barely starting leaves nothing to resume either.
func watchProgressAt(position, duration float64) domain.WatchProgress {
	position = math.Min(position, duration)
	p := domain.WatchProgress{PositionSeconds: position, DurationSeconds: duration}
	switch {
	case position > duration*WatchedFraction:
		p.Watched = true
		p.PositionSeconds = 0
	case position < WatchResumeMinSeconds:
		p.PositionSeconds = 0
	}
	return p
}

func validSeconds(v float64) bool {
	return v >= 0 && !math.IsInf(v, 0) && !math.IsNaN(v)
}
