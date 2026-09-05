package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	pgxmock "github.com/pashagolub/pgxmock/v3"
)

func TestCollectRowsPropagatesQueryError(t *testing.T) {
	boom := errors.New("boom")
	items, err := collectRows[int](nil, boom, func(pgx.Rows) (int, error) { return 0, nil })
	if !errors.Is(err, boom) || items != nil {
		t.Fatalf("expected query error to be returned, got items=%v err=%v", items, err)
	}
}

func TestCollectRowsScansEveryRowAndClosesRows(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	mock.ExpectQuery("SELECT n").WillReturnRows(pgxmock.NewRows([]string{"n"}).AddRow(1).AddRow(2).AddRow(3))
	rows, qerr := mock.Query(context.Background(), "SELECT n")
	got, err := collectRows(rows, qerr, func(rows pgx.Rows) (int, error) {
		var n int
		err := rows.Scan(&n)
		return n, err
	})
	if err != nil || len(got) != 3 || got[2] != 3 {
		t.Fatalf("unexpected result: %v %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCollectRowsReturnsScanError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	mock.ExpectQuery("SELECT n").WillReturnRows(pgxmock.NewRows([]string{"n"}).AddRow(1))
	rows, qerr := mock.Query(context.Background(), "SELECT n")
	scanErr := errors.New("bad scan")
	_, err = collectRows(rows, qerr, func(pgx.Rows) (int, error) { return 0, scanErr })
	if !errors.Is(err, scanErr) {
		t.Fatalf("expected scan error, got %v", err)
	}
}
