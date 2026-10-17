package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Dominux/Pentaract/internal/domain"
)

type UsersRepo struct {
	pool DB
}

func NewUsersRepo(pool DB) *UsersRepo {
	return &UsersRepo{pool: pool}
}

func scanUser(s scanner) (*domain.User, error) {
	user := &domain.User{}
	err := s.Scan(&user.ID, &user.Email, &user.PasswordHash)
	return user, err
}

func (r *UsersRepo) Create(ctx context.Context, email, passwordHash string) (*domain.User, error) {
	user, err := scanUser(r.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id, email, password_hash`,
		email, passwordHash,
	))
	if err != nil {
		return nil, conflictOr(err, "user")
	}
	return user, nil
}

func (r *UsersRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	user, err := scanUser(r.pool.QueryRow(ctx,
		`SELECT id, email, password_hash FROM users WHERE email = $1`,
		email,
	))
	if err != nil {
		return nil, notFoundOr(err, "user")
	}
	return user, nil
}

func (r *UsersRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	user, err := scanUser(r.pool.QueryRow(ctx,
		`SELECT id, email, password_hash FROM users WHERE id = $1`,
		id,
	))
	if err != nil {
		return nil, notFoundOr(err, "user")
	}
	return user, nil
}

func scanUserSummary(rows pgx.Rows) (domain.User, error) {
	var u domain.User
	err := rows.Scan(&u.ID, &u.Email)
	return u, err
}

func (r *UsersRepo) ListNonAdmin(ctx context.Context, adminEmail string) ([]domain.User, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, email FROM users WHERE LOWER(email) <> LOWER($1) ORDER BY email`,
		adminEmail,
	)
	return collectRows(rows, err, scanUserSummary)
}

func (r *UsersRepo) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	return execAffecting(ctx, r.pool, "user",
		`UPDATE users SET password_hash = $2 WHERE id = $1`,
		id, passwordHash,
	)
}

func (r *UsersRepo) DeleteManaged(ctx context.Context, id uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM access WHERE user_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM storage_workers WHERE user_id = $1`, id); err != nil {
		return err
	}
	if err := execAffecting(ctx, tx, "user", `DELETE FROM users WHERE id = $1`, id); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ListGrantCandidates returns users that can still receive access for a storage:
// excludes the caller and users already present in access for that storage.
func (r *UsersRepo) ListGrantCandidates(ctx context.Context, storageID, callerID uuid.UUID) ([]domain.User, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT u.id, u.email
		FROM users u
		WHERE u.id <> $2
		AND NOT EXISTS (
			SELECT 1 FROM access a
			WHERE a.storage_id = $1 AND a.user_id = u.id
		)
		ORDER BY u.email`,
		storageID, callerID,
	)
	return collectRows(rows, err, scanUserSummary)
}
