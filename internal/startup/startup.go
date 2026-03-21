package startup

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Dominux/Pentaract/internal/config"
	"github.com/Dominux/Pentaract/internal/password"
)

var pgxConnect = pgx.Connect

type StartupPool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

type createDBConn interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Close(ctx context.Context) error
}

func CreateDB(ctx context.Context, cfg *config.Config) error {
	conn, err := pgxConnect(ctx, cfg.DatabaseURLWithoutDB())
	if err != nil {
		return fmt.Errorf("connecting to postgres: %w", err)
	}
	defer conn.Close(ctx)

	return createDBWithConn(ctx, cfg, conn)
}

func createDBWithConn(ctx context.Context, cfg *config.Config, conn createDBConn) error {
	var exists bool
	err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)`, cfg.DatabaseName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("checking database existence: %w", err)
	}
	if exists {
		slog.Info("database already exists", "name", cfg.DatabaseName)
		return nil
	}

	safeName := pgx.Identifier{cfg.DatabaseName}.Sanitize()
	_, err = conn.Exec(ctx, "CREATE DATABASE "+safeName)
	if err != nil {
		return fmt.Errorf("creating database: %w", err)
	}
	slog.Info("database created", "name", cfg.DatabaseName)
	return nil
}

func InitDB(ctx context.Context, pool StartupPool) error {
	queries := []string{
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'access_type') THEN
				CREATE TYPE access_type AS ENUM ('r', 'w', 'a');
			END IF;
		END $$`,

		`CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email VARCHAR(255) UNIQUE NOT NULL,
			password_hash VARCHAR(255) NOT NULL
		)`,

		`CREATE TABLE IF NOT EXISTS storages (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name VARCHAR(255) NOT NULL,
			chat_id BIGINT UNIQUE NOT NULL
		)`,

		`CREATE TABLE IF NOT EXISTS storage_workers (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name VARCHAR(255) NOT NULL,
			user_id UUID NOT NULL REFERENCES users(id),
			token VARCHAR(255) UNIQUE NOT NULL,
			storage_id UUID REFERENCES storages(id) ON DELETE SET NULL
		)`,

		`CREATE TABLE IF NOT EXISTS access (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES users(id),
			storage_id UUID NOT NULL REFERENCES storages(id) ON DELETE CASCADE,
			access_type access_type NOT NULL,
			UNIQUE(user_id, storage_id)
		)`,

		`CREATE TABLE IF NOT EXISTS files (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			path TEXT NOT NULL,
			size BIGINT NOT NULL DEFAULT 0,
			storage_id UUID NOT NULL REFERENCES storages(id) ON DELETE CASCADE,
			is_uploaded BOOLEAN NOT NULL DEFAULT false
		)`,

		`CREATE TABLE IF NOT EXISTS file_chunks (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
			telegram_file_id TEXT NOT NULL,
			telegram_message_id BIGINT NOT NULL DEFAULT 0,
			position SMALLINT NOT NULL
		)`,

		// Migration: add telegram_message_id if missing (existing DBs)
		`DO $$ BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'file_chunks' AND column_name = 'telegram_message_id'
			) THEN
				ALTER TABLE file_chunks ADD COLUMN telegram_message_id BIGINT NOT NULL DEFAULT 0;
			END IF;
		END $$`,

		`CREATE TABLE IF NOT EXISTS storage_workers_usages (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			worker_id UUID NOT NULL REFERENCES storage_workers(id) ON DELETE CASCADE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,

		`CREATE INDEX IF NOT EXISTS storage_workers_usages_worker_created_at_idx
			ON storage_workers_usages (worker_id, created_at)`,

		`CREATE INDEX IF NOT EXISTS storage_workers_usages_created_at_idx
			ON storage_workers_usages (created_at)`,

		`CREATE OR REPLACE FUNCTION regexp_quote(text) RETURNS text AS $$
			SELECT regexp_replace($1, '([.?*+^$[\]\\(){}|\\-])', '\\\1', 'g');
		$$ LANGUAGE sql IMMUTABLE`,

		// Performance indexes for file queries
		`CREATE INDEX IF NOT EXISTS files_storage_id_path_idx ON files (storage_id, path)`,
		`CREATE INDEX IF NOT EXISTS files_storage_id_pending_idx ON files (storage_id) WHERE is_uploaded = false`,

		// Fix double slashes in file paths from previous bug
		`UPDATE files SET path = regexp_replace(path, '//', '/', 'g') WHERE path LIKE '%//%'`,
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, q := range queries {
		if _, err := tx.Exec(ctx, q); err != nil {
			return fmt.Errorf("executing migration: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func CreateSuperuser(ctx context.Context, pool StartupPool, cfg *config.Config) error {
	hash, err := password.Hash(cfg.SuperuserPass)
	if err != nil {
		return fmt.Errorf("hashing superuser password: %w", err)
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO users (email, password_hash) VALUES ($1, $2) ON CONFLICT (email) DO NOTHING`,
		cfg.SuperuserEmail, hash,
	)
	if err != nil {
		return fmt.Errorf("creating superuser: %w", err)
	}

	slog.Info("superuser ensured", "email", cfg.SuperuserEmail)
	return nil
}
