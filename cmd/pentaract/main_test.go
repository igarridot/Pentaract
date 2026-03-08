package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Dominux/Pentaract/internal/config"
)

var (
	origLoadConfigFn        = loadConfigFn
	origCreateDBFn          = createDBFn
	origParsePoolConfigFn   = parsePoolConfigFn
	origNewPoolWithConfigFn = newPoolWithConfigFn
	origInitDBFn            = initDBFn
	origCreateSuperuserFn   = createSuperuserFn
	origNewServerHandlerFn  = newServerHandlerFn
	origSignalNotifyFn      = signalNotifyFn
	origListenAndServeFn    = listenAndServeFn
)

func resetMainDeps() {
	loadConfigFn = origLoadConfigFn
	createDBFn = origCreateDBFn
	parsePoolConfigFn = origParsePoolConfigFn
	newPoolWithConfigFn = origNewPoolWithConfigFn
	initDBFn = origInitDBFn
	createSuperuserFn = origCreateSuperuserFn
	newServerHandlerFn = origNewServerHandlerFn
	signalNotifyFn = origSignalNotifyFn
	listenAndServeFn = origListenAndServeFn
}

func testCfg() *config.Config {
	return &config.Config{
		Port:                   8080,
		Workers:                1,
		SuperuserEmail:         "admin@example.com",
		SuperuserPass:          "secret",
		AccessTokenExpireInSec: 3600,
		SecretKey:              "secret",
		TelegramAPIBaseURL:     "http://localhost",
		TelegramRateLimit:      10,
		DatabaseUser:           "u",
		DatabasePassword:       "p",
		DatabaseName:           "db",
		DatabaseHost:           "localhost",
		DatabasePort:           5432,
	}
}

func TestRunSuccess(t *testing.T) {
	t.Cleanup(resetMainDeps)

	cfg := testCfg()
	loadConfigFn = func() *config.Config { return cfg }
	createDBFn = func(ctx context.Context, cfg *config.Config) error { return nil }
	parsePoolConfigFn = func(connString string) (*pgxpool.Config, error) { return &pgxpool.Config{}, nil }
	newPoolWithConfigFn = func(ctx context.Context, config *pgxpool.Config) (*pgxpool.Pool, error) { return nil, nil }
	initDBFn = func(ctx context.Context, pool *pgxpool.Pool) error { return nil }
	createSuperuserFn = func(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config) error { return nil }
	newServerHandlerFn = func(cfg *config.Config, pool *pgxpool.Pool) http.Handler { return http.NewServeMux() }
	signalNotifyFn = func(c chan<- os.Signal, sig ...os.Signal) {}
	listenAndServeFn = func(srv *http.Server) error { return http.ErrServerClosed }

	if err := run(context.Background()); err != nil {
		t.Fatalf("run should succeed, got: %v", err)
	}
}

func TestRunErrors(t *testing.T) {
	tests := []struct {
		name string
		seed func()
	}{
		{
			name: "create db",
			seed: func() {
				createDBFn = func(ctx context.Context, cfg *config.Config) error { return errors.New("x") }
			},
		},
		{
			name: "parse pool config",
			seed: func() {
				createDBFn = func(ctx context.Context, cfg *config.Config) error { return nil }
				parsePoolConfigFn = func(connString string) (*pgxpool.Config, error) { return nil, errors.New("x") }
			},
		},
		{
			name: "new pool",
			seed: func() {
				createDBFn = func(ctx context.Context, cfg *config.Config) error { return nil }
				parsePoolConfigFn = func(connString string) (*pgxpool.Config, error) { return &pgxpool.Config{}, nil }
				newPoolWithConfigFn = func(ctx context.Context, config *pgxpool.Config) (*pgxpool.Pool, error) { return nil, errors.New("x") }
			},
		},
		{
			name: "init db",
			seed: func() {
				createDBFn = func(ctx context.Context, cfg *config.Config) error { return nil }
				parsePoolConfigFn = func(connString string) (*pgxpool.Config, error) { return &pgxpool.Config{}, nil }
				newPoolWithConfigFn = func(ctx context.Context, config *pgxpool.Config) (*pgxpool.Pool, error) { return nil, nil }
				initDBFn = func(ctx context.Context, pool *pgxpool.Pool) error { return errors.New("x") }
			},
		},
		{
			name: "superuser",
			seed: func() {
				createDBFn = func(ctx context.Context, cfg *config.Config) error { return nil }
				parsePoolConfigFn = func(connString string) (*pgxpool.Config, error) { return &pgxpool.Config{}, nil }
				newPoolWithConfigFn = func(ctx context.Context, config *pgxpool.Config) (*pgxpool.Pool, error) { return nil, nil }
				initDBFn = func(ctx context.Context, pool *pgxpool.Pool) error { return nil }
				createSuperuserFn = func(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config) error { return errors.New("x") }
			},
		},
		{
			name: "listen and serve",
			seed: func() {
				createDBFn = func(ctx context.Context, cfg *config.Config) error { return nil }
				parsePoolConfigFn = func(connString string) (*pgxpool.Config, error) { return &pgxpool.Config{}, nil }
				newPoolWithConfigFn = func(ctx context.Context, config *pgxpool.Config) (*pgxpool.Pool, error) { return nil, nil }
				initDBFn = func(ctx context.Context, pool *pgxpool.Pool) error { return nil }
				createSuperuserFn = func(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config) error { return nil }
				newServerHandlerFn = func(cfg *config.Config, pool *pgxpool.Pool) http.Handler { return http.NewServeMux() }
				signalNotifyFn = func(c chan<- os.Signal, sig ...os.Signal) {}
				listenAndServeFn = func(srv *http.Server) error { return errors.New("x") }
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(resetMainDeps)
			loadConfigFn = func() *config.Config { return testCfg() }
			tt.seed()
			if err := run(context.Background()); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestMainEntryPointSuccess(t *testing.T) {
	t.Cleanup(resetMainDeps)

	cfg := testCfg()
	loadConfigFn = func() *config.Config { return cfg }
	createDBFn = func(ctx context.Context, cfg *config.Config) error { return nil }
	parsePoolConfigFn = func(connString string) (*pgxpool.Config, error) { return &pgxpool.Config{}, nil }
	newPoolWithConfigFn = func(ctx context.Context, config *pgxpool.Config) (*pgxpool.Pool, error) { return nil, nil }
	initDBFn = func(ctx context.Context, pool *pgxpool.Pool) error { return nil }
	createSuperuserFn = func(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config) error { return nil }
	newServerHandlerFn = func(cfg *config.Config, pool *pgxpool.Pool) http.Handler { return http.NewServeMux() }
	signalNotifyFn = func(c chan<- os.Signal, sig ...os.Signal) {}
	listenAndServeFn = func(srv *http.Server) error { return http.ErrServerClosed }

	main()
}
