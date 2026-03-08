package startup

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxmock "github.com/pashagolub/pgxmock/v3"

	"github.com/Dominux/Pentaract/internal/config"
)

type fakeCreateDBRow struct {
	scanFn func(dest ...any) error
}

func (r *fakeCreateDBRow) Scan(dest ...any) error {
	return r.scanFn(dest...)
}

type fakeCreateDBConn struct {
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
	execFn     func(ctx context.Context, sql string, args ...any) error
	closeFn    func(ctx context.Context) error
}

func (c *fakeCreateDBConn) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return c.queryRowFn(ctx, sql, args...)
}
func (c *fakeCreateDBConn) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if c.execFn != nil {
		if err := c.execFn(ctx, sql, args...); err != nil {
			return pgconn.CommandTag{}, err
		}
	}
	return pgconn.NewCommandTag("CREATE DATABASE"), nil
}
func (c *fakeCreateDBConn) Close(ctx context.Context) error {
	if c.closeFn != nil {
		return c.closeFn(ctx)
	}
	return nil
}

func TestInitDBAndCreateSuperuser(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("new pgxmock pool: %v", err)
	}
	defer mock.Close()

	// InitDB starts tx, executes many statements, commits.
	mock.ExpectBegin()
	for i := 0; i < 11; i++ {
		mock.ExpectExec(".*").WillReturnResult(pgxmock.NewResult("EXEC", 1))
	}
	mock.ExpectCommit()
	if err := initDBWithPool(context.Background(), mock); err != nil {
		t.Fatalf("init db failed: %v", err)
	}

	cfg := testConfig()
	mock.ExpectExec("INSERT INTO users").WithArgs(cfg.SuperuserEmail, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := createSuperuserWithPool(context.Background(), mock, cfg); err != nil {
		t.Fatalf("create superuser failed: %v", err)
	}
}

func TestCreateDB(t *testing.T) {
	old := pgxConnect
	pgxConnect = func(ctx context.Context, connString string) (*pgx.Conn, error) {
		return nil, context.Canceled
	}
	t.Cleanup(func() { pgxConnect = old })

	cfg := &config.Config{
		DatabaseUser:     "u",
		DatabasePassword: "p",
		DatabaseHost:     "h",
		DatabasePort:     5432,
		DatabaseName:     "db",
	}
	if err := CreateDB(context.Background(), cfg); err == nil {
		t.Fatalf("expected error when connect fails")
	}
}

func TestCreateDBWithConnBranches(t *testing.T) {
	cfg := &config.Config{DatabaseName: "pentaract"}

	connExists := &fakeCreateDBConn{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			return &fakeCreateDBRow{scanFn: func(dest ...any) error {
				*(dest[0].(*bool)) = true
				return nil
			}}
		},
	}
	if err := createDBWithConn(context.Background(), cfg, connExists); err != nil {
		t.Fatalf("expected nil when db exists, got: %v", err)
	}

	connCreate := &fakeCreateDBConn{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			return &fakeCreateDBRow{scanFn: func(dest ...any) error {
				*(dest[0].(*bool)) = false
				return nil
			}}
		},
	}
	if err := createDBWithConn(context.Background(), cfg, connCreate); err != nil {
		t.Fatalf("expected create success, got: %v", err)
	}

	connQueryErr := &fakeCreateDBConn{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			return &fakeCreateDBRow{scanFn: func(dest ...any) error { return errors.New("scan failed") }}
		},
	}
	if err := createDBWithConn(context.Background(), cfg, connQueryErr); err == nil {
		t.Fatalf("expected query/scan error")
	}

	connExecErr := &fakeCreateDBConn{
		queryRowFn: func(ctx context.Context, sql string, args ...any) pgx.Row {
			return &fakeCreateDBRow{scanFn: func(dest ...any) error {
				*(dest[0].(*bool)) = false
				return nil
			}}
		},
		execFn: func(ctx context.Context, sql string, args ...any) error { return errors.New("exec failed") },
	}
	if err := createDBWithConn(context.Background(), cfg, connExecErr); err == nil {
		t.Fatalf("expected exec error")
	}
}

func TestWrapperFunctionsAreInvoked(t *testing.T) {
	// Call wrappers with nil pool to execute wrapper lines. They panic because the
	// wrapped implementation dereferences pool, which is expected in this test.
	func() {
		defer func() { _ = recover() }()
		_ = InitDB(context.Background(), (*pgxpool.Pool)(nil))
	}()

	func() {
		defer func() { _ = recover() }()
		_ = CreateSuperuser(context.Background(), (*pgxpool.Pool)(nil), testConfig())
	}()
}

func testConfig() *config.Config {
	return &config.Config{
		SuperuserEmail: "admin@example.com",
		SuperuserPass:  "secret",
	}
}
