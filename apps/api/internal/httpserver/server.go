package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/raffinauvall/noxwatch/apps/api/internal/alerts"
	"github.com/raffinauvall/noxwatch/apps/api/internal/auth"
	"github.com/raffinauvall/noxwatch/apps/api/internal/config"
	"github.com/raffinauvall/noxwatch/apps/api/internal/enrollment"
	"github.com/raffinauvall/noxwatch/apps/api/internal/metrics"
	"github.com/raffinauvall/noxwatch/apps/api/internal/notifications"
	"github.com/raffinauvall/noxwatch/apps/api/internal/servers"
	"github.com/raffinauvall/noxwatch/apps/api/internal/workspaces"
)

type readyCheck func(context.Context) error

func New(cfg config.Config, logger *slog.Logger, db *pgxpool.Pool, ready readyCheck) *http.Server {
	mux := http.NewServeMux()
	platform := &platformMetrics{}
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("GET /ready", readyHandler(logger, ready))
	mux.HandleFunc("GET /metrics", platform.handler)
	authHandler := auth.NewHandler(auth.NewService(db, cfg.AuthSecret), cfg, logger)
	authHandler.RegisterRoutes(mux)
	workspaces.NewHandler(workspaces.NewService(db), logger).RegisterRoutes(mux, authHandler.Require)
	enrollmentService := enrollment.NewService(db)
	enrollment.NewHandler(enrollmentService, logger).RegisterRoutes(mux, authHandler.Require)
	notificationService := notifications.NewService(db, cfg.AuthSecret, cfg.PublicWebURL, cfg.AppEnv == "development")
	alertService := alerts.NewService(db, notificationService)
	alerts.NewHandler(alertService, logger).RegisterRoutes(mux, authHandler.Require)
	notifications.NewHandler(notificationService, cfg, logger).RegisterRoutes(mux, authHandler.Require)
	metrics.NewHandler(metrics.NewService(db, enrollmentService), logger, func(ctx context.Context, payload metrics.Payload) error {
		diskUsage := 0.0
		for _, disk := range payload.Disks {
			if disk.UsagePercent > diskUsage {
				diskUsage = disk.UsagePercent
			}
		}
		return alertService.EvaluateMetrics(ctx, payload.ServerID, payload.CollectedAt, alerts.Values{CPU: payload.CPU.UsagePercent, Memory: payload.Memory.UsagePercent, Disk: diskUsage, Swap: payload.Swap.UsagePercent})
	}).RegisterRoutes(mux, authHandler.Require)
	servers.NewHandler(servers.NewService(db), logger).RegisterRoutes(mux, authHandler.Require)

	handler := recoverer(logger)(requestLogger(logger, platform)(securityHeaders(cfg.AppEnv == "production")(cors(cfg.CORSAllowedOrigins)(bodyLimit(mux)))))
	return &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{
			"service": "noxwatch-api",
			"status":  "ok",
		},
		"error": nil,
	})
}

func readyHandler(logger *slog.Logger, ready readyCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := ready(ctx); err != nil {
			logger.Warn("readiness check failed", "request_id", requestID(r), "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"data": nil,
				"error": map[string]string{
					"code":       "SERVICE_UNAVAILABLE",
					"message":    "A required dependency is unavailable.",
					"request_id": requestID(r),
				},
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]string{
				"status": "ready",
			},
			"error": nil,
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func bodyLimit(next http.Handler) http.Handler {
	return http.MaxBytesHandler(next, 1<<20)
}

func cors(origins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && slices.Contains(origins, origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func securityHeaders(production bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			if production {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

func requestLogger(logger *slog.Logger, platform *platformMetrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-ID")
			if id == "" {
				id = newRequestID()
			}
			w.Header().Set("X-Request-ID", id)
			r.Header.Set("X-Request-ID", id)
			r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(rec, r)
			duration := time.Since(start)
			platform.requests.Add(1)
			platform.durationMilliseconds.Add(uint64(duration.Milliseconds()))
			if rec.status >= 500 {
				platform.errors.Add(1)
			}
			logger.Info("http_request",
				"request_id", id,
				"method", r.Method,
				"route", r.URL.Path,
				"status", rec.status,
				"duration_ms", duration.Milliseconds(),
			)
		})
	}
}

type platformMetrics struct {
	requests             atomic.Uint64
	errors               atomic.Uint64
	durationMilliseconds atomic.Uint64
}

func (m *platformMetrics) handler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write([]byte("# TYPE noxwatch_http_requests_total counter\n"))
	_, _ = w.Write([]byte("noxwatch_http_requests_total " + strconv.FormatUint(m.requests.Load(), 10) + "\n"))
	_, _ = w.Write([]byte("# TYPE noxwatch_http_errors_total counter\n"))
	_, _ = w.Write([]byte("noxwatch_http_errors_total " + strconv.FormatUint(m.errors.Load(), 10) + "\n"))
	_, _ = w.Write([]byte("# TYPE noxwatch_http_duration_milliseconds_total counter\n"))
	_, _ = w.Write([]byte("noxwatch_http_duration_milliseconds_total " + strconv.FormatUint(m.durationMilliseconds.Load(), 10) + "\n"))
}

func recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error("panic recovered", "request_id", requestID(r))
					writeJSON(w, http.StatusInternalServerError, map[string]any{
						"data": nil,
						"error": map[string]string{
							"code":       "INTERNAL_ERROR",
							"message":    "An unexpected error occurred.",
							"request_id": requestID(r),
						},
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

type requestIDKey struct{}

func requestID(r *http.Request) string {
	if id, ok := r.Context().Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}
	return "req_" + hex.EncodeToString(b[:])
}
