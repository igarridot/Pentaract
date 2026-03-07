package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port                   int
	Workers                int
	SuperuserEmail         string
	SuperuserPass          string
	AccessTokenExpireInSec int
	SecretKey              string
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

func Load() (*Config, error) {
	cfg := &Config{
		Port:                   getEnvInt("PORT", 8000),
		Workers:                getEnvInt("WORKERS", 4),
		SuperuserEmail:         mustGetEnv("SUPERUSER_EMAIL"),
		SuperuserPass:          mustGetEnv("SUPERUSER_PASS"),
		AccessTokenExpireInSec: getEnvInt("ACCESS_TOKEN_EXPIRE_IN_SECS", 31536000),
		SecretKey:              mustGetEnv("SECRET_KEY"),
		TelegramAPIBaseURL:     getEnv("TELEGRAM_API_BASE_URL", "https://api.telegram.org"),
		TelegramRateLimit:      getEnvInt("TELEGRAM_RATE_LIMIT", 18),
		DatabaseUser:           mustGetEnv("DATABASE_USER"),
		DatabasePassword:       mustGetEnv("DATABASE_PASSWORD"),
		DatabaseName:           mustGetEnv("DATABASE_NAME"),
		DatabaseHost:           getEnv("DATABASE_HOST", "db"),
		DatabasePort:           getEnvInt("DATABASE_PORT", 5432),
	}
	return cfg, nil
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
