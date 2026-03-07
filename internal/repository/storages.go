package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Dominux/Pentaract/internal/domain"
)

type StoragesRepo struct {
	pool *pgxpool.Pool
}

func NewStoragesRepo(pool *pgxpool.Pool) *StoragesRepo {
	return &StoragesRepo{pool: pool}
}

func (r *StoragesRepo) Create(ctx context.Context, name string, chatID int64) (*domain.Storage, error) {
	s := &domain.Storage{}
	err := r.pool.QueryRow(ctx,
		`INSERT INTO storages (name, chat_id) VALUES ($1, $2) RETURNING id, name, chat_id`,
		name, chatID,
	).Scan(&s.ID, &s.Name, &s.ChatID)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, domain.ErrAlreadyExists("storage")
		}
		return nil, err
	}
	return s, nil
}

func (r *StoragesRepo) List(ctx context.Context, userID uuid.UUID) ([]domain.StorageWithInfo, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT s.id, s.name, s.chat_id,
			COALESCE(COUNT(f.id), 0) AS files_amount,
			COALESCE(SUM(f.size), 0) AS size
		FROM storages s
		INNER JOIN access a ON a.storage_id = s.id
		LEFT JOIN files f ON f.storage_id = s.id AND f.is_uploaded = true
		WHERE a.user_id = $1
		GROUP BY s.id, s.name, s.chat_id
		ORDER BY s.name`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var storages []domain.StorageWithInfo
	for rows.Next() {
		var s domain.StorageWithInfo
		if err := rows.Scan(&s.ID, &s.Name, &s.ChatID, &s.FilesAmount, &s.Size); err != nil {
			return nil, err
		}
		storages = append(storages, s)
	}
	return storages, rows.Err()
}

func (r *StoragesRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Storage, error) {
	s := &domain.Storage{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, chat_id FROM storages WHERE id = $1`,
		id,
	).Scan(&s.ID, &s.Name, &s.ChatID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound("storage")
		}
		return nil, err
	}
	return s, nil
}

func (r *StoragesRepo) Delete(ctx context.Context, id uuid.UUID) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM storages WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrNotFound("storage")
	}
	return nil
}
