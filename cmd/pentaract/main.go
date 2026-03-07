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

func main() {
	ctx := context.Background()

	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create database if not exists
	if err := startup.CreateDB(ctx, cfg); err != nil {
		log.Fatalf("Failed to create database: %v", err)
	}

	// Connect to database
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL())
	if err != nil {
		log.Fatalf("Failed to parse database config: %v", err)
	}
	poolCfg.MaxConns = int32(cfg.Workers * 8)

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Run migrations
	if err := startup.InitDB(ctx, pool); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Create superuser
	if err := startup.CreateSuperuser(ctx, pool, cfg); err != nil {
		log.Fatalf("Failed to create superuser: %v", err)
	}

	// Build and start server
	handler := server.New(cfg, pool)

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
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		log.Println("Shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Printf("Server starting on port %d", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
