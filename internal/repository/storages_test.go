package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v3"
)

func TestStoragesRepoCRUD(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()
	repo := NewStoragesRepoWithDB(mock)

	sid := uuid.New()
	uid := uuid.New()

	mock.ExpectQuery("INSERT INTO storages").
		WithArgs("Main", int64(1)).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(sid, "Main", int64(1)))
	if _, err := repo.Create(context.Background(), "Main", 1); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	mock.ExpectQuery("SELECT s.id, s.name, s.chat_id").
		WithArgs(uid).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id", "files_amount", "size"}).AddRow(sid, "Main", int64(1), int64(0), int64(0)))
	if list, err := repo.List(context.Background(), uid); err != nil || len(list) != 1 {
		t.Fatalf("list failed: %v %v", list, err)
	}

	mock.ExpectQuery("SELECT id, name, chat_id FROM storages WHERE id = \\$1").
		WithArgs(sid).
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "chat_id"}).AddRow(sid, "Main", int64(1)))
	if _, err := repo.GetByID(context.Background(), sid); err != nil {
		t.Fatalf("get failed: %v", err)
	}

	mock.ExpectExec("DELETE FROM storages WHERE id = \\$1").WithArgs(sid).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	if err := repo.Delete(context.Background(), sid); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
}
