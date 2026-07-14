package config

import (
	"strings"
	"testing"
)

func TestLoadRequiresExternalServices(t *testing.T) {
	_, err := Load(func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") || !strings.Contains(err.Error(), "REDIS_ADDR") {
		t.Fatalf("expected missing env error, got %v", err)
	}
}

func TestLoadParsesAllowedOrigins(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":          "postgres://user:pass@localhost/db",
		"REDIS_ADDR":            "localhost:6379",
		"AUTH_SECRET":           strings.Repeat("x", 32),
		"CORS_ALLOWED_ORIGINS":  "http://localhost:3000, https://app.example.com ",
		"METRIC_RETENTION_DAYS": "14",
	}
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(cfg.CORSAllowedOrigins, ","); got != "http://localhost:3000,https://app.example.com" {
		t.Fatalf("unexpected origins: %s", got)
	}
	if cfg.MetricRetentionDays != 14 {
		t.Fatalf("retention days=%d", cfg.MetricRetentionDays)
	}
}
