package repository

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Dominux/Pentaract/internal/domain"
)

type StorageWorkersRepo struct {
	pool                storageWorkersDB
	usageCleanupMu      sync.Mutex
	lastUsageCleanup    time.Time
	usageCleanupRunning bool
}

const (
	storageWorkerUsageWindow          = time.Minute
	storageWorkerUsageCleanupInterval = 5 * time.Minute
)

type storageWorkersDB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

func NewStorageWorkersRepo(pool storageWorkersDB) *StorageWorkersRepo {
	return &StorageWorkersRepo{
		pool:             pool,
		lastUsageCleanup: time.Now(),
	}
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
		`SELECT EXISTS(SELECT 1 FROM storage_workers WHERE storage_id = $1 OR storage_id IS NULL)`,
		storageID,
	).Scan(&exists)
	return exists, err
}

// ListTokensByStorage returns all worker tokens assigned to a storage.
func (r *StorageWorkersRepo) ListTokensByStorage(ctx context.Context, storageID uuid.UUID) ([]WorkerToken, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT token, name FROM storage_workers
		WHERE storage_id = $1 OR storage_id IS NULL
		ORDER BY name`,
		storageID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]WorkerToken, 0)
	for rows.Next() {
		var wt WorkerToken
		if err := rows.Scan(&wt.Token, &wt.Name); err != nil {
			return nil, err
		}
		tokens = append(tokens, wt)
	}
	return tokens, rows.Err()
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

// Update updates a worker's name and storage assignment.
func (r *StorageWorkersRepo) Update(ctx context.Context, id, userID uuid.UUID, name string, storageID *uuid.UUID) (*domain.StorageWorker, error) {
	w := &domain.StorageWorker{}
	err := r.pool.QueryRow(ctx,
		`UPDATE storage_workers SET name = $3, storage_id = $4
		WHERE id = $1 AND user_id = $2
		RETURNING id, name, user_id, token, storage_id`,
		id, userID, name, storageID,
	).Scan(&w.ID, &w.Name, &w.UserID, &w.Token, &w.StorageID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound("storage worker")
		}
		return nil, err
	}
	return w, nil
}

// WorkerToken holds the token and name of a selected worker.
type WorkerToken struct {
	Token string
	Name  string
}

func (r *StorageWorkersRepo) scheduleUsageCleanup() {
	r.usageCleanupMu.Lock()
	if r.usageCleanupRunning || time.Since(r.lastUsageCleanup) < storageWorkerUsageCleanupInterval {
		r.usageCleanupMu.Unlock()
		return
	}
	r.usageCleanupRunning = true
	r.lastUsageCleanup = time.Now()
	r.usageCleanupMu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if _, err := r.pool.Exec(ctx,
			`DELETE FROM storage_workers_usages WHERE created_at < now() - interval '1 minute'`,
		); err != nil {
			slog.Warn("cleanup of expired worker usages failed", "err", err)
		}

		r.usageCleanupMu.Lock()
		r.usageCleanupRunning = false
		r.usageCleanupMu.Unlock()
	}()
}

// GetToken atomically selects the least-loaded worker under the rate limit
// and records a usage entry. Returns nil if no worker is available.
func (r *StorageWorkersRepo) GetToken(ctx context.Context, storageID uuid.UUID, rateLimit int) (*WorkerToken, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Select least-loaded worker under rate limit and insert usage
	var token, name string
	err = tx.QueryRow(ctx,
		`WITH available_workers AS (
			SELECT sw.id, sw.name, sw.token, COUNT(swu.id) AS usage_count
			FROM storage_workers sw
			LEFT JOIN storage_workers_usages swu
				ON swu.worker_id = sw.id
				AND swu.created_at >= now() - interval '1 minute'
			WHERE sw.storage_id = $1 OR sw.storage_id IS NULL
			GROUP BY sw.id, sw.name, sw.token
			HAVING COUNT(swu.id) < $2
			ORDER BY COUNT(swu.id) ASC, sw.id
			LIMIT 1
		),
		inserted AS (
			INSERT INTO storage_workers_usages (worker_id)
			SELECT id FROM available_workers
			RETURNING worker_id
		)
		SELECT aw.token, aw.name
		FROM available_workers aw
		JOIN inserted i ON i.worker_id = aw.id`,
		storageID, rateLimit,
	).Scan(&token, &name)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			r.scheduleUsageCleanup()
			return nil, nil
		}
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	r.scheduleUsageCleanup()
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
		WHERE sw.storage_id = $1 OR sw.storage_id IS NULL
		AND swu.created_at >= now() - interval '1 minute'`,
		storageID,
	).Scan(&earliest)
	if err != nil || earliest.IsZero() {
		return 0, err
	}
	// The slot opens when this entry is older than 1 minute
	available := earliest.Add(storageWorkerUsageWindow).Sub(time.Now())
	if available < 0 {
		return 0, nil
	}
	// Add a small buffer to avoid racing
	return available + 100*time.Millisecond, nil
}
