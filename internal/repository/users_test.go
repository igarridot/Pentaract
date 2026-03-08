package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	pgxmock "github.com/pashagolub/pgxmock/v3"
)

func TestUsersRepoCreateAndGet(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	repo := NewUsersRepoWithDB(mock)
	uid := uuid.New()

	mock.ExpectQuery("INSERT INTO users").
		WithArgs("u@example.com", "hash").
		WillReturnRows(pgxmock.NewRows([]string{"id", "email", "password_hash"}).AddRow(uid, "u@example.com", "hash"))

	user, err := repo.Create(context.Background(), "u@example.com", "hash")
	if err != nil || user.ID != uid {
		t.Fatalf("create failed: user=%+v err=%v", user, err)
	}

	mock.ExpectQuery("SELECT id, email, password_hash FROM users WHERE email = \\$1").
		WithArgs("u@example.com").
		WillReturnRows(pgxmock.NewRows([]string{"id", "email", "password_hash"}).AddRow(uid, "u@example.com", "hash"))

	got, err := repo.GetByEmail(context.Background(), "u@example.com")
	if err != nil || got.Email != "u@example.com" {
		t.Fatalf("get by email failed: user=%+v err=%v", got, err)
	}

	mock.ExpectQuery("SELECT id, email, password_hash FROM users WHERE id = \\$1").
		WithArgs(uid).
		WillReturnRows(pgxmock.NewRows([]string{"id", "email", "password_hash"}).AddRow(uid, "u@example.com", "hash"))
	if _, err := repo.GetByID(context.Background(), uid); err != nil {
		t.Fatalf("get by id failed: %v", err)
	}
}

func TestUsersRepoListAndMutations(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	repo := NewUsersRepoWithDB(mock)
	uid := uuid.New()

	mock.ExpectQuery("SELECT id, email FROM users WHERE LOWER\\(email\\) <> LOWER\\(\\$1\\) ORDER BY email").
		WithArgs("admin@example.com").
		WillReturnRows(pgxmock.NewRows([]string{"id", "email"}).AddRow(uid, "u@example.com"))
	users, err := repo.ListNonAdmin(context.Background(), "admin@example.com")
	if err != nil || len(users) != 1 {
		t.Fatalf("list non-admin failed: users=%v err=%v", users, err)
	}

	mock.ExpectExec("UPDATE users SET password_hash = \\$2 WHERE id = \\$1").
		WithArgs(uid, "newhash").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	if err := repo.UpdatePassword(context.Background(), uid, "newhash"); err != nil {
		t.Fatalf("update password failed: %v", err)
	}
}

func TestUsersRepoDeleteManagedAndCandidates(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	repo := NewUsersRepoWithDB(mock)
	uid := uuid.New()
	sid := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM access WHERE user_id = \\$1").WithArgs(uid).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec("DELETE FROM storage_workers WHERE user_id = \\$1").WithArgs(uid).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec("DELETE FROM users WHERE id = \\$1").WithArgs(uid).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectCommit()
	if err := repo.DeleteManaged(context.Background(), uid); err != nil {
		t.Fatalf("delete managed failed: %v", err)
	}

	mock.ExpectQuery("SELECT u.id, u.email").
		WithArgs(sid, uid).
		WillReturnRows(pgxmock.NewRows([]string{"id", "email"}).AddRow(uuid.New(), "candidate@example.com"))
	candidates, err := repo.ListGrantCandidates(context.Background(), sid, uid)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("list candidates failed: %v %v", candidates, err)
	}
}

func TestUsersRepoErrorBranches(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()
	repo := NewUsersRepoWithDB(mock)
	uid := uuid.New()

	mock.ExpectQuery("INSERT INTO users").
		WithArgs("u@example.com", "hash").
		WillReturnError(&pgconn.PgError{Code: "23505"})
	if _, err := repo.Create(context.Background(), "u@example.com", "hash"); err == nil {
		t.Fatalf("expected create unique violation")
	}

	mock.ExpectQuery("SELECT id, email, password_hash FROM users WHERE email = \\$1").
		WithArgs("missing@example.com").
		WillReturnError(pgx.ErrNoRows)
	if _, err := repo.GetByEmail(context.Background(), "missing@example.com"); err == nil {
		t.Fatalf("expected get by email not found")
	}

	mock.ExpectQuery("SELECT id, email, password_hash FROM users WHERE id = \\$1").
		WithArgs(uid).
		WillReturnError(pgx.ErrNoRows)
	if _, err := repo.GetByID(context.Background(), uid); err == nil {
		t.Fatalf("expected get by id not found")
	}

	mock.ExpectExec("UPDATE users SET password_hash = \\$2 WHERE id = \\$1").
		WithArgs(uid, "newhash").
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	if err := repo.UpdatePassword(context.Background(), uid, "newhash"); err == nil {
		t.Fatalf("expected update password not found")
	}

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM access WHERE user_id = \\$1").WithArgs(uid).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec("DELETE FROM storage_workers WHERE user_id = \\$1").WithArgs(uid).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec("DELETE FROM users WHERE id = \\$1").WithArgs(uid).WillReturnResult(pgxmock.NewResult("DELETE", 0))
	if err := repo.DeleteManaged(context.Background(), uid); err == nil {
		t.Fatalf("expected delete managed not found")
	}
}
