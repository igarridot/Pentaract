package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Dominux/Pentaract/internal/domain"
)

type StoragesRepo struct {
	pool DB
}

func NewStoragesRepo(pool DB) *StoragesRepo {
	return &StoragesRepo{pool: pool}
}

func scanStorage(s scanner) (*domain.Storage, error) {
	st := &domain.Storage{}
	err := s.Scan(&st.ID, &st.Name, &st.ChatID)
	return st, err
}

func (r *StoragesRepo) Create(ctx context.Context, name string, chatID int64) (*domain.Storage, error) {
	s, err := scanStorage(r.pool.QueryRow(ctx,
		`INSERT INTO storages (name, chat_id) VALUES ($1, $2) RETURNING id, name, chat_id`,
		name, chatID,
	))
	if err != nil {
		return nil, conflictOr(err, "storage")
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
	return collectRows(rows, err, func(rows pgx.Rows) (domain.StorageWithInfo, error) {
		var s domain.StorageWithInfo
		err := rows.Scan(&s.ID, &s.Name, &s.ChatID, &s.FilesAmount, &s.Size)
		return s, err
	})
}

func (r *StoragesRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Storage, error) {
	s, err := scanStorage(r.pool.QueryRow(ctx,
		`SELECT id, name, chat_id FROM storages WHERE id = $1`,
		id,
	))
	if err != nil {
		return nil, notFoundOr(err, "storage")
	}
	return s, nil
}

func (r *StoragesRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return execAffecting(ctx, r.pool, "storage", `DELETE FROM storages WHERE id = $1`, id)
}
