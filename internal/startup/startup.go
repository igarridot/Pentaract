package startup

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Dominux/Pentaract/internal/config"
	"github.com/Dominux/Pentaract/internal/password"
)

func CreateDB(ctx context.Context, cfg *config.Config) error {
	conn, err := pgx.Connect(ctx, cfg.DatabaseURLWithoutDB())
	if err != nil {
		return fmt.Errorf("connecting to postgres: %w", err)
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", cfg.DatabaseName))
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			log.Printf("Database %s already exists", cfg.DatabaseName)
			return nil
		}
		return fmt.Errorf("creating database: %w", err)
	}

	log.Printf("Database %s created", cfg.DatabaseName)
	return nil
}

func InitDB(ctx context.Context, pool *pgxpool.Pool) error {
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

		`CREATE OR REPLACE FUNCTION regexp_quote(text) RETURNS text AS $$
			SELECT regexp_replace($1, '([.?*+^$[\]\\(){}|\\-])', '\\\1', 'g');
		$$ LANGUAGE sql IMMUTABLE`,

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

func CreateSuperuser(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config) error {
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

	log.Printf("Superuser %s ensured", cfg.SuperuserEmail)
	return nil
}
