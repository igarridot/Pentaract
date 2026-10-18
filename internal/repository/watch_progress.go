package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
)

// WatchProgressRepo keeps one row per (file, user): where the user stopped
// and whether they finished it. Saving updates that row in place, and the
// row goes away with its file or its user (ON DELETE CASCADE).
type WatchProgressRepo struct {
	pool DB
}

func NewWatchProgressRepo(pool DB) *WatchProgressRepo {
	return &WatchProgressRepo{pool: pool}
}

// Save stores the user's progress on a file. Watched never goes back to
// false: watching a finished video again only moves its resume point.
func (r *WatchProgressRepo) Save(ctx context.Context, userID, fileID uuid.UUID, p domain.WatchProgress) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO watch_progress (file_id, user_id, position_seconds, duration_seconds, watched, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (file_id, user_id) DO UPDATE SET
			position_seconds = EXCLUDED.position_seconds,
			duration_seconds = EXCLUDED.duration_seconds,
			watched = watch_progress.watched OR EXCLUDED.watched,
			updated_at = EXCLUDED.updated_at`,
		fileID, userID, p.PositionSeconds, p.DurationSeconds, p.Watched,
	)
	return err
}

// SetWatched marks a file as watched or not, by hand, and returns what is
// stored. Unlike Save it can clear Watched, and it drops the resume point
// either way: a finished video, or one to watch again from the start.
func (r *WatchProgressRepo) SetWatched(ctx context.Context, userID, fileID uuid.UUID, watched bool) (*domain.WatchProgress, error) {
	p := &domain.WatchProgress{}
	err := r.pool.QueryRow(ctx,
		`INSERT INTO watch_progress (file_id, user_id, position_seconds, watched, updated_at)
		VALUES ($1, $2, 0, $3, now())
		ON CONFLICT (file_id, user_id) DO UPDATE SET
			position_seconds = 0,
			watched = EXCLUDED.watched,
			updated_at = EXCLUDED.updated_at
		RETURNING position_seconds, duration_seconds, watched, updated_at`,
		fileID, userID, watched,
	).Scan(&p.PositionSeconds, &p.DurationSeconds, &p.Watched, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// ListByPaths returns the user's progress on the given files of a storage,
// keyed by path. Files the user never played are absent.
func (r *WatchProgressRepo) ListByPaths(ctx context.Context, userID, storageID uuid.UUID, paths []string) (map[string]domain.WatchProgress, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT f.path, w.position_seconds, w.duration_seconds, w.watched, w.updated_at
		FROM watch_progress w
		JOIN files f ON f.id = w.file_id
		WHERE w.user_id = $1 AND f.storage_id = $2 AND f.path = ANY($3)`,
		userID, storageID, paths,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	progress := make(map[string]domain.WatchProgress)
	for rows.Next() {
		var path string
		var p domain.WatchProgress
		if err := rows.Scan(&path, &p.PositionSeconds, &p.DurationSeconds, &p.Watched, &p.UpdatedAt); err != nil {
			return nil, err
		}
		progress[path] = p
	}
	return progress, rows.Err()
}

// DeleteAll forgets everything the user has played, in every storage, and
// returns how many rows went.
func (r *WatchProgressRepo) DeleteAll(ctx context.Context, userID uuid.UUID) (int64, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM watch_progress WHERE user_id = $1`, userID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// DeleteUnder forgets what the user has played among the files of a storage
// whose path starts with dirPrefix (a folder ending in "/", every subfolder
// included), and returns how many rows went. starts_with rather than LIKE so
// "%" and "_" in folder names are not wildcards.
func (r *WatchProgressRepo) DeleteUnder(ctx context.Context, userID, storageID uuid.UUID, dirPrefix string) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM watch_progress w
		USING files f
		WHERE f.id = w.file_id AND w.user_id = $1 AND f.storage_id = $2 AND starts_with(f.path, $3)`,
		userID, storageID, dirPrefix,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
