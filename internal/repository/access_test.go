package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v3"

	"github.com/Dominux/Pentaract/internal/domain"
)

func TestAccessRepoCRUD(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	repo := NewAccessRepoWithDB(mock)
	uid := uuid.New()
	sid := uuid.New()

	mock.ExpectExec("INSERT INTO access").
		WithArgs(uid, sid, domain.AccessAdmin).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := repo.CreateOrUpdate(context.Background(), uid, sid, domain.AccessAdmin); err != nil {
		t.Fatalf("createOrUpdate failed: %v", err)
	}

	mock.ExpectQuery("SELECT u.id, u.email, a.access_type").
		WithArgs(sid).
		WillReturnRows(pgxmock.NewRows([]string{"id", "email", "access_type"}).AddRow(uid, "u@example.com", domain.AccessAdmin))
	users, err := repo.List(context.Background(), sid)
	if err != nil || len(users) != 1 {
		t.Fatalf("list failed: %v users=%v", err, users)
	}

	mock.ExpectExec("DELETE FROM access WHERE user_id = \\$1 AND storage_id = \\$2").
		WithArgs(uid, sid).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	if err := repo.Delete(context.Background(), uid, sid); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
}

func TestAccessRepoHasAccessAndDeleteNotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	repo := NewAccessRepoWithDB(mock)
	uid := uuid.New()
	sid := uuid.New()

	mock.ExpectQuery("SELECT EXISTS\\(").
		WithArgs(uid, sid).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	has, err := repo.HasAccess(context.Background(), uid, sid, domain.AccessRead)
	if err != nil || !has {
		t.Fatalf("hasAccess failed: has=%v err=%v", has, err)
	}

	mock.ExpectExec("DELETE FROM access WHERE user_id = \\$1 AND storage_id = \\$2").
		WithArgs(uid, sid).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))
	if err := repo.Delete(context.Background(), uid, sid); err == nil {
		t.Fatalf("expected not found on zero rows")
	}
}
