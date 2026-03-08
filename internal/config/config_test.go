package config

import (
	"strings"
	"testing"
)

func requiredEnv() map[string]string {
	return map[string]string{
		"SUPERUSER_EMAIL":   "admin@example.com",
		"SUPERUSER_PASS":    "secret",
		"SECRET_KEY":        "abc123",
		"DATABASE_USER":     "dbuser",
		"DATABASE_PASSWORD": "dbpass",
		"DATABASE_NAME":     "pentaract",
	}
}

func setRequiredEnv(t *testing.T) {
	t.Helper()
	for k, v := range requiredEnv() {
		t.Setenv(k, v)
	}
}

func mustPanicContains(t *testing.T, fn func(), contains string) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic containing %q", contains)
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected panic string, got %T", r)
		}
		if !strings.Contains(msg, contains) {
			t.Fatalf("panic %q does not contain %q", msg, contains)
		}
	}()
	fn()
}

func TestLoadDefaultsAndURLs(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Port != 8000 || cfg.Workers != 4 {
		t.Fatalf("unexpected defaults: port=%d workers=%d", cfg.Port, cfg.Workers)
	}
	if cfg.TelegramAPIBaseURL != "https://api.telegram.org" {
		t.Fatalf("unexpected TelegramAPIBaseURL: %q", cfg.TelegramAPIBaseURL)
	}
	if cfg.TelegramRateLimit != 18 {
		t.Fatalf("unexpected TelegramRateLimit: %d", cfg.TelegramRateLimit)
	}
	if cfg.DatabaseHost != "db" || cfg.DatabasePort != 5432 {
		t.Fatalf("unexpected db defaults: host=%q port=%d", cfg.DatabaseHost, cfg.DatabasePort)
	}
	if cfg.DatabaseURL() != "postgres://dbuser:dbpass@db:5432/pentaract?sslmode=disable" {
		t.Fatalf("unexpected DatabaseURL: %q", cfg.DatabaseURL())
	}
	if cfg.DatabaseURLWithoutDB() != "postgres://dbuser:dbpass@db:5432/postgres?sslmode=disable" {
		t.Fatalf("unexpected DatabaseURLWithoutDB: %q", cfg.DatabaseURLWithoutDB())
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PORT", "9001")
	t.Setenv("WORKERS", "7")
	t.Setenv("ACCESS_TOKEN_EXPIRE_IN_SECS", "1234")
	t.Setenv("TELEGRAM_API_BASE_URL", "https://example.test")
	t.Setenv("TELEGRAM_RATE_LIMIT", "11")
	t.Setenv("DATABASE_HOST", "localhost")
	t.Setenv("DATABASE_PORT", "15432")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.Port != 9001 || cfg.Workers != 7 || cfg.AccessTokenExpireInSec != 1234 {
		t.Fatalf("unexpected env overrides: %+v", cfg)
	}
	if cfg.TelegramAPIBaseURL != "https://example.test" || cfg.TelegramRateLimit != 11 {
		t.Fatalf("unexpected telegram overrides: %+v", cfg)
	}
	if cfg.DatabaseHost != "localhost" || cfg.DatabasePort != 15432 {
		t.Fatalf("unexpected db overrides: %+v", cfg)
	}
}

func TestLoadPanicsOnMissingRequiredEnv(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SUPERUSER_EMAIL", "")
	mustPanicContains(t, func() { _, _ = Load() }, "required environment variable SUPERUSER_EMAIL is not set")
}

func TestLoadPanicsOnInvalidInteger(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PORT", "not-an-int")
	mustPanicContains(t, func() { _, _ = Load() }, "environment variable PORT must be an integer")
}
