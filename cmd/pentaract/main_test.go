package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Dominux/Pentaract/internal/config"
	"github.com/Dominux/Pentaract/internal/startup"
)

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

func happyDeps(cfg *config.Config) runDeps {
	return runDeps{
		loadConfig:        func() *config.Config { return cfg },
		createDB:          func(ctx context.Context, cfg *config.Config) error { return nil },
		parsePoolConfig:   func(connString string) (*pgxpool.Config, error) { return &pgxpool.Config{}, nil },
		newPoolWithConfig: func(ctx context.Context, config *pgxpool.Config) (*pgxpool.Pool, error) { return nil, nil },
		initDB:            func(ctx context.Context, pool startup.StartupPool) error { return nil },
		createSuperuser:   func(ctx context.Context, pool startup.StartupPool, cfg *config.Config) error { return nil },
		newServerHandler:  func(cfg *config.Config, pool *pgxpool.Pool) http.Handler { return http.NewServeMux() },
		signalNotify:      func(c chan<- os.Signal, sig ...os.Signal) {},
		listenAndServe:    func(srv *http.Server) error { return http.ErrServerClosed },
	}
}

func TestRunSuccess(t *testing.T) {
	if err := run(context.Background(), happyDeps(testCfg())); err != nil {
		t.Fatalf("run should succeed, got: %v", err)
	}
}

func TestRunErrors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(d *runDeps)
	}{
		{
			name: "create db",
			mutate: func(d *runDeps) {
				d.createDB = func(ctx context.Context, cfg *config.Config) error { return errors.New("x") }
			},
		},
		{
			name: "parse pool config",
			mutate: func(d *runDeps) {
				d.parsePoolConfig = func(connString string) (*pgxpool.Config, error) { return nil, errors.New("x") }
			},
		},
		{
			name: "new pool",
			mutate: func(d *runDeps) {
				d.newPoolWithConfig = func(ctx context.Context, config *pgxpool.Config) (*pgxpool.Pool, error) { return nil, errors.New("x") }
			},
		},
		{
			name: "init db",
			mutate: func(d *runDeps) {
				d.initDB = func(ctx context.Context, pool startup.StartupPool) error { return errors.New("x") }
			},
		},
		{
			name: "superuser",
			mutate: func(d *runDeps) {
				d.createSuperuser = func(ctx context.Context, pool startup.StartupPool, cfg *config.Config) error { return errors.New("x") }
			},
		},
		{
			name: "listen and serve",
			mutate: func(d *runDeps) {
				d.listenAndServe = func(srv *http.Server) error { return errors.New("x") }
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := happyDeps(testCfg())
			tt.mutate(&deps)
			if err := run(context.Background(), deps); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestMainEntryPointSuccess(t *testing.T) {
	// Cannot test main() directly without global state;
	// main() calls run(ctx, defaultDeps()) which requires real infra.
	// The run() function is fully covered above.
	t.Skip("main() requires real infrastructure; run() is tested directly")
}
