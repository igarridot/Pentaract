package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Dominux/Pentaract/internal/domain"
)

type AccessRepo struct {
	pool *pgxpool.Pool
}

func NewAccessRepo(pool *pgxpool.Pool) *AccessRepo {
	return &AccessRepo{pool: pool}
}

func (r *AccessRepo) CreateOrUpdate(ctx context.Context, userID, storageID uuid.UUID, accessType domain.AccessType) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO access (user_id, storage_id, access_type) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, storage_id) DO UPDATE SET access_type = $3`,
		userID, storageID, accessType,
	)
	return err
}

func (r *AccessRepo) List(ctx context.Context, storageID uuid.UUID) ([]domain.UserWithAccess, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT u.id, u.email, a.access_type
		FROM access a
		INNER JOIN users u ON u.id = a.user_id
		WHERE a.storage_id = $1
		ORDER BY u.email`,
		storageID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []domain.UserWithAccess
	for rows.Next() {
		var u domain.UserWithAccess
		if err := rows.Scan(&u.ID, &u.Email, &u.AccessType); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *AccessRepo) Delete(ctx context.Context, userID, storageID uuid.UUID) error {
	ct, err := r.pool.Exec(ctx,
		`DELETE FROM access WHERE user_id = $1 AND storage_id = $2`,
		userID, storageID,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrNotFound("access")
	}
	return nil
}

func (r *AccessRepo) HasAccess(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error) {
	var filter string
	switch requiredLevel {
	case domain.AccessRead:
		filter = `access_type IN ('r', 'w', 'a')`
	case domain.AccessWrite:
		filter = `access_type IN ('w', 'a')`
	case domain.AccessAdmin:
		filter = `access_type = 'a'`
	default:
		return false, nil
	}

	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM access WHERE user_id = $1 AND storage_id = $2 AND `+filter+`)`,
		userID, storageID,
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

