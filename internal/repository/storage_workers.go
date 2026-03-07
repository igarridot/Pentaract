package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Dominux/Pentaract/internal/domain"
)

type StorageWorkersRepo struct {
	pool *pgxpool.Pool
}

func NewStorageWorkersRepo(pool *pgxpool.Pool) *StorageWorkersRepo {
	return &StorageWorkersRepo{pool: pool}
}

func (r *StorageWorkersRepo) Create(ctx context.Context, name string, userID uuid.UUID, token string, storageID *uuid.UUID) (*domain.StorageWorker, error) {
	w := &domain.StorageWorker{}
	err := r.pool.QueryRow(ctx,
		`INSERT INTO storage_workers (name, user_id, token, storage_id) VALUES ($1, $2, $3, $4)
		RETURNING id, name, user_id, token, storage_id`,
		name, userID, token, storageID,
	).Scan(&w.ID, &w.Name, &w.UserID, &w.Token, &w.StorageID)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, domain.ErrAlreadyExists("storage worker")
		}
		return nil, err
	}
	return w, nil
}

func (r *StorageWorkersRepo) List(ctx context.Context, userID uuid.UUID) ([]domain.StorageWorker, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, user_id, token, storage_id FROM storage_workers WHERE user_id = $1 ORDER BY name`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workers []domain.StorageWorker
	for rows.Next() {
		var w domain.StorageWorker
		if err := rows.Scan(&w.ID, &w.Name, &w.UserID, &w.Token, &w.StorageID); err != nil {
			return nil, err
		}
		workers = append(workers, w)
	}
	return workers, rows.Err()
}

func (r *StorageWorkersRepo) HasWorkers(ctx context.Context, storageID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM storage_workers WHERE storage_id = $1)`,
		storageID,
	).Scan(&exists)
	return exists, err
}

func (r *StorageWorkersRepo) Delete(ctx context.Context, id, userID uuid.UUID) error {
	ct, err := r.pool.Exec(ctx,
		`DELETE FROM storage_workers WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrNotFound("storage worker")
	}
	return nil
}

// WorkerToken holds the token and name of a selected worker.
type WorkerToken struct {
	Token string
	Name  string
}

// GetToken atomically selects the least-loaded worker under the rate limit
// and records a usage entry. Returns nil if no worker is available.
func (r *StorageWorkersRepo) GetToken(ctx context.Context, storageID uuid.UUID, rateLimit int) (*WorkerToken, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Clean up old usage records (older than 1 minute)
	_, err = tx.Exec(ctx,
		`DELETE FROM storage_workers_usages WHERE created_at < now() - interval '1 minute'`,
	)
	if err != nil {
		return nil, err
	}

	// Select least-loaded worker under rate limit and insert usage
	var token, name string
	err = tx.QueryRow(ctx,
		`WITH available_workers AS (
			SELECT sw.id, sw.name, sw.token, COUNT(swu.id) AS usage_count
			FROM storage_workers sw
			LEFT JOIN storage_workers_usages swu ON swu.worker_id = sw.id
			WHERE sw.storage_id = $1
			GROUP BY sw.id, sw.name, sw.token
			HAVING COUNT(swu.id) < $2
			ORDER BY COUNT(swu.id) ASC
			LIMIT 1
		),
		inserted AS (
			INSERT INTO storage_workers_usages (worker_id)
			SELECT id FROM available_workers
			RETURNING worker_id
		)
		SELECT aw.token, aw.name FROM available_workers aw`,
		storageID, rateLimit,
	).Scan(&token, &name)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &WorkerToken{Token: token, Name: name}, nil
}

// NextAvailableIn returns the duration until the next worker slot becomes available.
// It finds the oldest usage entry that, when it expires (after 1 minute), frees a slot.
func (r *StorageWorkersRepo) NextAvailableIn(ctx context.Context, storageID uuid.UUID, rateLimit int) (time.Duration, error) {
	// Find the earliest created_at among workers that are at or above the rate limit.
	// That entry expires after 1 minute, freeing a slot.
	var earliest time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT MIN(swu.created_at)
		FROM storage_workers_usages swu
		JOIN storage_workers sw ON sw.id = swu.worker_id
		WHERE sw.storage_id = $1
		AND swu.created_at >= now() - interval '1 minute'`,
		storageID,
	).Scan(&earliest)
	if err != nil || earliest.IsZero() {
		return 0, err
	}
	// The slot opens when this entry is older than 1 minute
	available := earliest.Add(time.Minute).Sub(time.Now())
	if available < 0 {
		return 0, nil
	}
	// Add a small buffer to avoid racing
	return available + 100*time.Millisecond, nil
}
