package config

import (
	"errors"
	"fmt"
	"strings"
)

type Config struct {
	AppEnv             string
	HTTPAddr           string
	DatabaseURL        string
	RedisAddr          string
	AuthSecret         string
	CORSAllowedOrigins []string
	MigrationsDir      string
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		AppEnv:        value(getenv, "APP_ENV", "development"),
		HTTPAddr:      value(getenv, "HTTP_ADDR", ":8080"),
		DatabaseURL:   getenv("DATABASE_URL"),
		RedisAddr:     getenv("REDIS_ADDR"),
		AuthSecret:    getenv("AUTH_SECRET"),
		MigrationsDir: value(getenv, "MIGRATIONS_DIR", "../../migrations"),
	}
	if origins := strings.TrimSpace(getenv("CORS_ALLOWED_ORIGINS")); origins != "" {
		for _, origin := range strings.Split(origins, ",") {
			if trimmed := strings.TrimSpace(origin); trimmed != "" {
				cfg.CORSAllowedOrigins = append(cfg.CORSAllowedOrigins, trimmed)
			}
		}
	}
	return cfg, cfg.Validate()
}

func (c Config) Validate() error {
	var missing []string
	if c.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if c.RedisAddr == "" {
		missing = append(missing, "REDIS_ADDR")
	}
	if len(c.AuthSecret) < 32 {
		missing = append(missing, "AUTH_SECRET (at least 32 characters)")
	}
	if c.MigrationsDir == "" {
		missing = append(missing, "MIGRATIONS_DIR")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required env: %s", strings.Join(missing, ", "))
	}
	if c.AppEnv == "production" && strings.Contains(c.DatabaseURL, "sslmode=disable") {
		return errors.New("production DATABASE_URL must not disable TLS")
	}
	return nil
}

func value(getenv func(string) string, key, fallback string) string {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		return v
	}
	return fallback
}
