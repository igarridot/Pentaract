package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v3"
)

func TestStorageWorkersRepoBasic(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()
	repo := NewStorageWorkersRepoWithDB(mock)

	userID := uuid.New()
	storageID := uuid.New()
	workerID := uuid.New()

	mock.ExpectQuery("INSERT INTO storage_workers").
		WithArgs("w1", userID, "token", &storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "user_id", "token", "storage_id"}).AddRow(workerID, "w1", userID, "token", nil))
	if _, err := repo.Create(context.Background(), "w1", userID, "token", &storageID); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	mock.ExpectQuery("SELECT id, name, user_id, token, storage_id FROM storage_workers WHERE user_id = \\$1 ORDER BY name").
		WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "user_id", "token", "storage_id"}).AddRow(workerID, "w1", userID, "token", nil))
	if workers, err := repo.List(context.Background(), userID); err != nil || len(workers) != 1 {
		t.Fatalf("list failed: workers=%v err=%v", workers, err)
	}

	mock.ExpectQuery("SELECT EXISTS\\(SELECT 1 FROM storage_workers WHERE storage_id = \\$1 OR storage_id IS NULL\\)").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	if has, err := repo.HasWorkers(context.Background(), storageID); err != nil || !has {
		t.Fatalf("hasWorkers failed: %v %v", has, err)
	}

	mock.ExpectQuery("SELECT token, name FROM storage_workers").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"token", "name"}).AddRow("token", "w1"))
	if tokens, err := repo.ListTokensByStorage(context.Background(), storageID); err != nil || len(tokens) != 1 {
		t.Fatalf("list tokens failed: %v %v", tokens, err)
	}
}

func TestStorageWorkersRepoMutationsAndScheduling(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()
	repo := NewStorageWorkersRepoWithDB(mock)

	userID := uuid.New()
	storageID := uuid.New()
	workerID := uuid.New()

	mock.ExpectQuery("UPDATE storage_workers SET name = \\$3, storage_id = \\$4").
		WithArgs(workerID, userID, "w2", &storageID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "user_id", "token", "storage_id"}).AddRow(workerID, "w2", userID, "token", nil))
	if _, err := repo.Update(context.Background(), workerID, userID, "w2", &storageID); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	mock.ExpectExec("DELETE FROM storage_workers WHERE id = \\$1 AND user_id = \\$2").
		WithArgs(workerID, userID).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	if err := repo.Delete(context.Background(), workerID, userID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM storage_workers_usages").WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectQuery("WITH available_workers AS").
		WithArgs(storageID, 10).
		WillReturnRows(pgxmock.NewRows([]string{"token", "name"}).AddRow("token", "w1"))
	mock.ExpectCommit()
	if wt, err := repo.GetToken(context.Background(), storageID, 10); err != nil || wt == nil {
		t.Fatalf("get token failed: %v %v", wt, err)
	}

	mock.ExpectQuery("SELECT MIN\\(swu.created_at\\)").
		WithArgs(storageID).
		WillReturnRows(pgxmock.NewRows([]string{"min"}).AddRow(time.Now()))
	if d, err := repo.NextAvailableIn(context.Background(), storageID, 10); err != nil || d < 0 {
		t.Fatalf("next available failed: %v %v", d, err)
	}
}
