package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
)

const defaultDBMaxConns = 32

type Config struct {
	Port                   int
	DBMaxConns             int
	SuperuserEmail         string
	SuperuserPass          string
	AccessTokenExpireInSec int
	SecretKey              string
	EncryptionKey          string
	TelegramAPIBaseURL     string
	TelegramRateLimit      int
	DatabaseUser           string
	DatabasePassword       string
	DatabaseName           string
	DatabaseHost           string
	DatabasePort           int
}

func (c *Config) DatabaseURL() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		c.DatabaseUser, c.DatabasePassword, c.DatabaseHost, c.DatabasePort, c.DatabaseName)
}

func (c *Config) DatabaseURLWithoutDB() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/postgres?sslmode=disable",
		c.DatabaseUser, c.DatabasePassword, c.DatabaseHost, c.DatabasePort)
}

// ChunkCipherSecrets returns the secret that seals new chunks and the legacy
// secret that must still open old ones. Without ENCRYPTION_KEY everything is
// derived from SECRET_KEY, exactly as before the variable existed; once
// ENCRYPTION_KEY is set, SECRET_KEY stays as the fallback so files uploaded
// earlier keep decrypting.
func (c *Config) ChunkCipherSecrets() (secret, legacy string) {
	if c.EncryptionKey == "" {
		return c.SecretKey, ""
	}
	return c.EncryptionKey, c.SecretKey
}

func Load() *Config {
	return &Config{
		Port:                   getEnvInt("PORT", 8000),
		DBMaxConns:             loadDBMaxConns(),
		SuperuserEmail:         mustGetEnv("SUPERUSER_EMAIL"),
		SuperuserPass:          mustGetEnv("SUPERUSER_PASS"),
		AccessTokenExpireInSec: getEnvInt("ACCESS_TOKEN_EXPIRE_IN_SECS", 31536000),
		SecretKey:              mustGetEnv("SECRET_KEY"),
		EncryptionKey:          getEnv("ENCRYPTION_KEY", ""),
		TelegramAPIBaseURL:     getEnv("TELEGRAM_API_BASE_URL", "https://api.telegram.org"),
		TelegramRateLimit:      getEnvInt("TELEGRAM_RATE_LIMIT", 18),
		DatabaseUser:           mustGetEnv("DATABASE_USER"),
		DatabasePassword:       mustGetEnv("DATABASE_PASSWORD"),
		DatabaseName:           mustGetEnv("DATABASE_NAME"),
		DatabaseHost:           getEnv("DATABASE_HOST", "db"),
		DatabasePort:           getEnvInt("DATABASE_PORT", 5432),
	}
}

// loadDBMaxConns reads DB_MAX_CONNS. The old WORKERS variable (a multiplier of
// 8 connections) is still honoured so existing .env files keep working.
func loadDBMaxConns() int {
	if os.Getenv("DB_MAX_CONNS") != "" {
		return getEnvInt("DB_MAX_CONNS", defaultDBMaxConns)
	}
	if os.Getenv("WORKERS") != "" {
		slog.Warn("WORKERS is deprecated, set DB_MAX_CONNS instead (WORKERS * 8 connections)")
		return getEnvInt("WORKERS", 4) * 8
	}
	return defaultDBMaxConns
}

func mustGetEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		panic(fmt.Sprintf("required environment variable %s is not set", key))
	}
	return val
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		panic(fmt.Sprintf("environment variable %s must be an integer", key))
	}
	return n
}
