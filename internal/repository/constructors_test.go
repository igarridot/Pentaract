package repository

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryConstructors(t *testing.T) {
	var pool *pgxpool.Pool

	if NewUsersRepo(pool) == nil {
		t.Fatalf("NewUsersRepo returned nil")
	}
	if NewStoragesRepo(pool) == nil {
		t.Fatalf("NewStoragesRepo returned nil")
	}
	if NewAccessRepo(pool) == nil {
		t.Fatalf("NewAccessRepo returned nil")
	}
	if NewStorageWorkersRepo(pool) == nil {
		t.Fatalf("NewStorageWorkersRepo returned nil")
	}
	if NewFilesRepo(pool) == nil {
		t.Fatalf("NewFilesRepo returned nil")
	}
}
