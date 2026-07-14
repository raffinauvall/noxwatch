package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/raffinauvall/noxwatch/apps/api/internal/config"
)

func TestHealth(t *testing.T) {
	srv := New(testConfig(), slog.Default(), func(context.Context) error { return nil })
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestReadyFailureIsGeneric(t *testing.T) {
	srv := New(testConfig(), slog.Default(), func(context.Context) error { return errors.New("sql password leaked") })
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "SERVICE_UNAVAILABLE") || strings.Contains(body, "sql password leaked") {
		t.Fatalf("unsafe readiness response: %s", body)
	}
}

func testConfig() config.Config {
	return config.Config{
		HTTPAddr:           ":0",
		DatabaseURL:        "postgres://example",
		RedisAddr:          "localhost:6379",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
		MigrationsDir:      "../../migrations",
	}
}
