package main

import (
	"context"
	"fmt"
	"log"
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

var (
	loadConfigFn        = config.Load
	createDBFn          = startup.CreateDB
	parsePoolConfigFn   = pgxpool.ParseConfig
	newPoolWithConfigFn = pgxpool.NewWithConfig
	initDBFn            = startup.InitDB
	createSuperuserFn   = startup.CreateSuperuser
	newServerHandlerFn  = server.New
	signalNotifyFn      = signal.Notify
	listenAndServeFn    = func(srv *http.Server) error { return srv.ListenAndServe() }
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatalf("%v", err)
	}
}

func run(ctx context.Context) error {
	cfg := loadConfigFn()

	// Create database if not exists
	if err := createDBFn(ctx, cfg); err != nil {
		return fmt.Errorf("Failed to create database: %v", err)
	}

	// Connect to database
	poolCfg, err := parsePoolConfigFn(cfg.DatabaseURL())
	if err != nil {
		return fmt.Errorf("Failed to parse database config: %v", err)
	}
	poolCfg.MaxConns = int32(cfg.Workers * 8)

	pool, err := newPoolWithConfigFn(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("Failed to connect to database: %v", err)
	}
	if pool != nil {
		defer pool.Close()
	}

	// Run migrations
	if err := initDBFn(ctx, pool); err != nil {
		return fmt.Errorf("Failed to initialize database: %v", err)
	}

	// Create superuser
	if err := createSuperuserFn(ctx, pool, cfg); err != nil {
		return fmt.Errorf("Failed to create superuser: %v", err)
	}

	// Build and start server
	handler := newServerHandlerFn(cfg, pool)

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
		signalNotifyFn(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		log.Println("Shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("Server starting on port %d", cfg.Port)
	if err := listenAndServeFn(srv); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("Server error: %v", err)
	}
	return nil
}
