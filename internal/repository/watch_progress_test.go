package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v5"

	"github.com/Dominux/Pentaract/internal/domain"
)

func TestWatchProgressRepoSaveUpsertsOneRow(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	repo := NewWatchProgressRepo(mock)
	uid, fid := uuid.New(), uuid.New()
	p := domain.WatchProgress{PositionSeconds: 120, DurationSeconds: 3600}

	// Updating in place (not appending) and never un-watching are what keep
	// the table one row per file and user.
	mock.ExpectExec(`INSERT INTO watch_progress .* ON CONFLICT \(file_id, user_id\) DO UPDATE SET .*watched = watch_progress.watched OR EXCLUDED.watched`).
		WithArgs(fid, uid, 120.0, 3600.0, false).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := repo.Save(context.Background(), uid, fid, p); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	mock.ExpectExec("INSERT INTO watch_progress").
		WithArgs(fid, uid, 120.0, 3600.0, false).
		WillReturnError(errors.New("boom"))
	if err := repo.Save(context.Background(), uid, fid, p); err == nil {
		t.Fatalf("expected the database error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWatchProgressRepoListByPaths(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	repo := NewWatchProgressRepo(mock)
	uid, sid := uuid.New(), uuid.New()
	paths := []string{"a.mkv", "b.mkv"}
	updated := time.Now()

	mock.ExpectQuery(`SELECT f.path, w.position_seconds, w.duration_seconds, w.watched, w.updated_at\s+FROM watch_progress w\s+JOIN files f ON f.id = w.file_id`).
		WithArgs(uid, sid, paths).
		WillReturnRows(pgxmock.NewRows([]string{"path", "position_seconds", "duration_seconds", "watched", "updated_at"}).
			AddRow("a.mkv", 42.0, 100.0, false, updated))
	got, err := repo.ListByPaths(context.Background(), uid, sid, paths)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(got) != 1 || got["a.mkv"].PositionSeconds != 42 || !got["a.mkv"].UpdatedAt.Equal(updated) {
		t.Fatalf("unexpected progress: %+v", got)
	}

	mock.ExpectQuery("SELECT f.path").WithArgs(uid, sid, paths).WillReturnError(errors.New("boom"))
	if _, err := repo.ListByPaths(context.Background(), uid, sid, paths); err == nil {
		t.Fatalf("expected the database error")
	}
}

func TestWatchProgressRepoSetWatchedOverridesAndClearsResume(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	repo := NewWatchProgressRepo(mock)
	uid, fid := uuid.New(), uuid.New()
	updated := time.Now()

	// Unlike Save, a hand mark sets watched outright (so it can clear it)
	// and always drops the resume point.
	mock.ExpectQuery(`INSERT INTO watch_progress .* ON CONFLICT \(file_id, user_id\) DO UPDATE SET\s+position_seconds = 0,\s+watched = EXCLUDED.watched,.*RETURNING position_seconds, duration_seconds, watched, updated_at`).
		WithArgs(fid, uid, false).
		WillReturnRows(pgxmock.NewRows([]string{"position_seconds", "duration_seconds", "watched", "updated_at"}).
			AddRow(0.0, 3600.0, false, updated))
	got, err := repo.SetWatched(context.Background(), uid, fid, false)
	if err != nil {
		t.Fatalf("set watched failed: %v", err)
	}
	if got.Watched || got.PositionSeconds != 0 || got.DurationSeconds != 3600 || !got.UpdatedAt.Equal(updated) {
		t.Fatalf("unexpected progress: %+v", got)
	}

	mock.ExpectQuery("INSERT INTO watch_progress").WithArgs(fid, uid, true).WillReturnError(errors.New("boom"))
	if _, err := repo.SetWatched(context.Background(), uid, fid, true); err == nil {
		t.Fatalf("expected the database error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
