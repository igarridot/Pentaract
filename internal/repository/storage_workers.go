package repository

import (
	"context"
	"errors"

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

// GetToken atomically selects the least-loaded worker under the rate limit
// and records a usage entry. Returns empty string if no worker is available.
func (r *StorageWorkersRepo) GetToken(ctx context.Context, storageID uuid.UUID, rateLimit int) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	// Clean up old usage records (older than 1 minute)
	_, err = tx.Exec(ctx,
		`DELETE FROM storage_workers_usages WHERE created_at < now() - interval '1 minute'`,
	)
	if err != nil {
		return "", err
	}

	// Select least-loaded worker under rate limit and insert usage
	var token string
	err = tx.QueryRow(ctx,
		`WITH available_workers AS (
			SELECT sw.id, sw.token, COUNT(swu.id) AS usage_count
			FROM storage_workers sw
			LEFT JOIN storage_workers_usages swu ON swu.worker_id = sw.id
			WHERE sw.storage_id = $1
			GROUP BY sw.id, sw.token
			HAVING COUNT(swu.id) < $2
			ORDER BY COUNT(swu.id) ASC
			LIMIT 1
		),
		inserted AS (
			INSERT INTO storage_workers_usages (worker_id)
			SELECT id FROM available_workers
			RETURNING worker_id
		)
		SELECT aw.token FROM available_workers aw`,
		storageID, rateLimit,
	).Scan(&token)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return token, nil
}
