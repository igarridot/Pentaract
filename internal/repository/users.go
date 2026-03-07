package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Dominux/Pentaract/internal/domain"
)

type UsersRepo struct {
	pool *pgxpool.Pool
}

func NewUsersRepo(pool *pgxpool.Pool) *UsersRepo {
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

