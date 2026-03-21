package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Dominux/Pentaract/internal/domain"
)

type UsersRepo struct {
	pool usersDB
}

type usersDB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

func NewUsersRepo(pool usersDB) *UsersRepo {
	return &UsersRepo{pool: pool}
}

func (r *UsersRepo) Create(ctx context.Context, email, passwordHash string) (*domain.User, error) {
	user := &domain.User{}
	err := r.pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id, email, password_hash`,
		email, passwordHash,
	).Scan(&user.ID, &user.Email, &user.PasswordHash)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, domain.ErrAlreadyExists("user")
		}
		return nil, err
	}
	return user, nil
}

func (r *UsersRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	user := &domain.User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, password_hash FROM users WHERE email = $1`,
		email,
	).Scan(&user.ID, &user.Email, &user.PasswordHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound("user")
		}
		return nil, err
	}
	return user, nil
}

func (r *UsersRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	user := &domain.User{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, password_hash FROM users WHERE id = $1`,
		id,
	).Scan(&user.ID, &user.Email, &user.PasswordHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound("user")
		}
		return nil, err
	}
	return user, nil
}

func (r *UsersRepo) ListNonAdmin(ctx context.Context, adminEmail string) ([]domain.User, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, email FROM users WHERE LOWER(email) <> LOWER($1) ORDER BY email`,
		adminEmail,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]domain.User, 0)
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.Email); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *UsersRepo) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	ct, err := r.pool.Exec(ctx,
		`UPDATE users SET password_hash = $2 WHERE id = $1`,
		id, passwordHash,
	)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrNotFound("user")
	}
	return nil
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
	ct, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrNotFound("user")
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
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]domain.User, 0)
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.Email); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
