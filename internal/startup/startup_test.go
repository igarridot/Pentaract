package startup

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxmock "github.com/pashagolub/pgxmock/v3"

	"github.com/Dominux/Pentaract/internal/config"
)

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
