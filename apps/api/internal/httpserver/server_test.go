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
	srv := New(testConfig(), slog.Default(), nil, func(context.Context) error { return nil })
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
	srv := New(testConfig(), slog.Default(), nil, func(context.Context) error { return errors.New("sql password leaked") })
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

func TestPrometheusMetrics(t *testing.T) {
	srv := New(testConfig(), slog.Default(), nil, func(context.Context) error { return nil })
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "noxwatch_http_requests_total") {
		t.Fatalf("unexpected metrics response: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCORSAllowsPut(t *testing.T) {
	srv := New(testConfig(), slog.Default(), nil, func(context.Context) error { return nil })
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/servers/example/tunnel", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || !strings.Contains(rec.Header().Get("Access-Control-Allow-Methods"), "PUT") {
		t.Fatalf("unexpected CORS response: %d %q", rec.Code, rec.Header().Get("Access-Control-Allow-Methods"))
	}
}

func testConfig() config.Config {
	return config.Config{
		HTTPAddr:           ":0",
		DatabaseURL:        "postgres://example",
		RedisAddr:          "localhost:6379",
		AuthSecret:         strings.Repeat("x", 32),
		CORSAllowedOrigins: []string{"http://localhost:3000"},
		MigrationsDir:      "../../migrations",
	}
}
