package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Config struct {
	AppEnv              string
	HTTPAddr            string
	DatabaseURL         string
	RedisAddr           string
	AuthSecret          string
	PublicWebURL        string
	CORSAllowedOrigins  []string
	MigrationsDir       string
	MetricRetentionDays int
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		AppEnv:              value(getenv, "APP_ENV", "development"),
		HTTPAddr:            value(getenv, "HTTP_ADDR", ":8080"),
		DatabaseURL:         getenv("DATABASE_URL"),
		RedisAddr:           getenv("REDIS_ADDR"),
		AuthSecret:          getenv("AUTH_SECRET"),
		PublicWebURL:        value(getenv, "PUBLIC_WEB_URL", "http://localhost:3000"),
		MigrationsDir:       value(getenv, "MIGRATIONS_DIR", "../../migrations"),
		MetricRetentionDays: 30,
	}
	if raw := strings.TrimSpace(getenv("METRIC_RETENTION_DAYS")); raw != "" {
		days, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("METRIC_RETENTION_DAYS must be an integer")
		}
		cfg.MetricRetentionDays = days
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
	if c.MetricRetentionDays < 7 || c.MetricRetentionDays > 365 {
		return errors.New("METRIC_RETENTION_DAYS must be between 7 and 365")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required env: %s", strings.Join(missing, ", "))
	}
	if c.AppEnv == "production" && strings.Contains(c.DatabaseURL, "sslmode=disable") {
		return errors.New("production DATABASE_URL must not disable TLS")
	}
	if c.AppEnv == "production" && !strings.HasPrefix(c.PublicWebURL, "https://") {
		return errors.New("production PUBLIC_WEB_URL must use HTTPS")
	}
	return nil
}

func value(getenv func(string) string, key, fallback string) string {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		return v
	}
	return fallback
}
