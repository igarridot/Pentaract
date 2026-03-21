package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Dominux/Pentaract/internal/config"
	"github.com/Dominux/Pentaract/internal/server"
	"github.com/Dominux/Pentaract/internal/startup"
)

type runDeps struct {
	loadConfig        func() *config.Config
	createDB          func(ctx context.Context, cfg *config.Config) error
	parsePoolConfig   func(connString string) (*pgxpool.Config, error)
	newPoolWithConfig func(ctx context.Context, cfg *pgxpool.Config) (*pgxpool.Pool, error)
	initDB            func(ctx context.Context, pool startup.StartupPool) error
	createSuperuser   func(ctx context.Context, pool startup.StartupPool, cfg *config.Config) error
	newServerHandler  func(cfg *config.Config, pool *pgxpool.Pool) http.Handler
	signalNotify      func(c chan<- os.Signal, sig ...os.Signal)
	listenAndServe    func(srv *http.Server) error
}

func defaultDeps() runDeps {
	return runDeps{
		loadConfig:        config.Load,
		createDB:          startup.CreateDB,
		parsePoolConfig:   pgxpool.ParseConfig,
		newPoolWithConfig: pgxpool.NewWithConfig,
		initDB:            startup.InitDB,
		createSuperuser:   startup.CreateSuperuser,
		newServerHandler:  server.New,
		signalNotify:      signal.Notify,
		listenAndServe:    func(srv *http.Server) error { return srv.ListenAndServe() },
	}
}

func main() {
	if err := run(context.Background(), defaultDeps()); err != nil {
		log.Fatalf("%v", err)
	}
}

func run(ctx context.Context, deps runDeps) error {
	cfg := deps.loadConfig()

	// Create database if not exists
	if err := deps.createDB(ctx, cfg); err != nil {
		return fmt.Errorf("Failed to create database: %v", err)
	}

	// Connect to database
	poolCfg, err := deps.parsePoolConfig(cfg.DatabaseURL())
	if err != nil {
		return fmt.Errorf("Failed to parse database config: %v", err)
	}
	poolCfg.MaxConns = int32(cfg.Workers * 8)

	pool, err := deps.newPoolWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("Failed to connect to database: %v", err)
	}
	if pool != nil {
		defer pool.Close()
	}

	// Run migrations
	if err := deps.initDB(ctx, pool); err != nil {
		return fmt.Errorf("Failed to initialize database: %v", err)
	}

	// Create superuser
	if err := deps.createSuperuser(ctx, pool, cfg); err != nil {
		return fmt.Errorf("Failed to create superuser: %v", err)
	}

	// Build and start server
	handler := deps.newServerHandler(cfg, pool)

	srv := &http.Server{
		Addr:              fmt.Sprintf("0.0.0.0:%d", cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 30 * time.Second,
		// No ReadTimeout/WriteTimeout: large file uploads and downloads
		// can take hours depending on file size and Telegram rate limits.
		// Per-request timeouts are handled via context cancellation.
		IdleTimeout: 120 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		deps.signalNotify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		slog.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	slog.Info("server starting", "port", cfg.Port)
	if err := deps.listenAndServe(srv); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("Server error: %v", err)
	}
	return nil
}
