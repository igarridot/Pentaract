package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Dominux/Pentaract/internal/domain"
)

// DB is the subset of *pgxpool.Pool the repositories use. Tests satisfy it
// with pgxmock.
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

// execer is what both DB and pgx.Tx offer for statements.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// scanner is satisfied by pgx.Row and pgx.Rows, so one scan function can
// serve QueryRow and collectRows alike.
type scanner interface {
	Scan(dest ...any) error
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// notFoundOr turns a missing row into the domain's not-found error for what
// and leaves any other error untouched.
func notFoundOr(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound(what)
	}
	return err
}

// conflictOr turns a unique violation into the domain's already-exists error
// for what and leaves any other error untouched.
func conflictOr(err error, what string) error {
	if isUniqueViolation(err) {
		return domain.ErrAlreadyExists(what)
	}
	return err
}

// execAffecting runs a statement that must change at least one row, and
// reports what as not found when it changes none.
func execAffecting(ctx context.Context, db execer, what, sql string, args ...any) error {
	ct, err := db.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return domain.ErrNotFound(what)
	}
	return nil
}

// collectRows drains a Query result through scan, closing the rows. It takes
// the (rows, err) pair returned by Query so callers can pass it straight in.
func collectRows[T any](rows pgx.Rows, err error, scan func(pgx.Rows) (T, error)) ([]T, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []T
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
