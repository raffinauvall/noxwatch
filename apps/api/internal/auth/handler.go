package auth

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/raffinauvall/noxwatch/apps/api/internal/config"
	"github.com/raffinauvall/noxwatch/apps/api/internal/httpx"
)

type contextKey struct{}

type Handler struct {
	service *Service
	logger  *slog.Logger
	cfg     config.Config
	limiter *rateLimiter
}

func NewHandler(service *Service, cfg config.Config, logger *slog.Logger) *Handler {
	return &Handler{service: service, cfg: cfg, logger: logger, limiter: newRateLimiter(10, time.Minute)}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/register", h.rateLimit(h.register))
	mux.HandleFunc("POST /api/v1/auth/login", h.rateLimit(h.login))
	mux.HandleFunc("POST /api/v1/auth/refresh", h.refresh)
	mux.Handle("POST /api/v1/auth/logout", h.Require(http.HandlerFunc(h.logout)))
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	if !httpx.Decode(w, r, &input) {
		return
	}
	if fields := validateCredentials(input.Email, input.Password, input.Name); len(fields) > 0 {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "The request contains invalid fields.", fields)
		return
	}
	result, err := h.service.Register(r.Context(), input.Email, input.Password, input.Name, r.UserAgent(), clientIP(r))
	if errors.Is(err, ErrEmailExists) {
		httpx.WriteError(w, r, http.StatusBadRequest, "REGISTRATION_FAILED", "Unable to create account with these details.", nil)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.setRefreshCookie(w, result.RefreshToken)
	httpx.Write(w, http.StatusCreated, result)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !httpx.Decode(w, r, &input) {
		return
	}
	result, err := h.service.Login(r.Context(), input.Email, input.Password, r.UserAgent(), clientIP(r))
	if errors.Is(err, ErrInvalidCredentials) {
		httpx.WriteError(w, r, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "Invalid email or password.", nil)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.setRefreshCookie(w, result.RefreshToken)
	httpx.Write(w, http.StatusOK, result)
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	if !h.validOrigin(r) {
		httpx.WriteError(w, r, http.StatusForbidden, "CSRF_REJECTED", "The request origin is not allowed.", nil)
		return
	}
	cookie, err := r.Cookie("nox_refresh")
	if err != nil {
		httpx.WriteError(w, r, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "Authentication is required.", nil)
		return
	}
	result, err := h.service.Refresh(r.Context(), cookie.Value)
	if errors.Is(err, ErrInvalidSession) {
		h.clearRefreshCookie(w)
		httpx.WriteError(w, r, http.StatusUnauthorized, "SESSION_INVALID", "The session is invalid or expired.", nil)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.setRefreshCookie(w, result.RefreshToken)
	httpx.Write(w, http.StatusOK, result)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	claims, _ := Claims(r.Context())
	if err := h.service.Revoke(r.Context(), claims.SessionID, claims.UserID); err != nil && !errors.Is(err, ErrInvalidSession) {
		h.internalError(w, r, err)
		return
	}
	h.clearRefreshCookie(w)
	httpx.Write(w, http.StatusOK, map[string]bool{"revoked": true})
}

func (h *Handler) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.Fields(r.Header.Get("Authorization"))
		if len(header) != 2 || !strings.EqualFold(header[0], "Bearer") {
			httpx.WriteError(w, r, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "Authentication is required.", nil)
			return
		}
		claims, err := h.service.ValidateAccess(r.Context(), header[1])
		if err != nil {
			httpx.WriteError(w, r, http.StatusUnauthorized, "SESSION_INVALID", "The session is invalid or expired.", nil)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, claims)))
	})
}

func Claims(ctx context.Context) (AccessClaims, bool) {
	claims, ok := ctx.Value(contextKey{}).(AccessClaims)
	return claims, ok
}

func (h *Handler) setRefreshCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: "nox_refresh", Value: token, Path: "/api/v1/auth", HttpOnly: true,
		Secure: h.cfg.AppEnv == "production", SameSite: http.SameSiteStrictMode, MaxAge: int(refreshLifetime.Seconds()), Expires: time.Now().Add(refreshLifetime)})
}

func (h *Handler) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "nox_refresh", Path: "/api/v1/auth", HttpOnly: true,
		Secure: h.cfg.AppEnv == "production", SameSite: http.SameSiteStrictMode, MaxAge: -1})
}

func (h *Handler) validOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	return origin == "" || slices.Contains(h.cfg.CORSAllowedOrigins, origin)
}

func (h *Handler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("request failed", "request_id", r.Header.Get("X-Request-ID"), "error", err)
	httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred.", nil)
}

func validateCredentials(email, password, name string) map[string]string {
	fields := map[string]string{}
	address, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil || !strings.EqualFold(address.Address, strings.TrimSpace(email)) || len(email) > 254 {
		fields["email"] = "Enter a valid email address."
	}
	if len(password) < 12 || len(password) > 128 {
		fields["password"] = "Password must be between 12 and 128 characters."
	}
	if len(strings.TrimSpace(name)) > 100 {
		fields["name"] = "Name must not exceed 100 characters."
	}
	return fields
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return ""
}

func (h *Handler) rateLimit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.limiter.Allow(r.URL.Path + ":" + clientIP(r)) {
			w.Header().Set("Retry-After", "60")
			httpx.WriteError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "Too many attempts. Try again later.", nil)
			return
		}
		next(w, r)
	}
}

type rateEntry struct {
	count int
	start time.Time
}

type rateLimiter struct {
	mu     sync.Mutex
	items  map[string]rateEntry
	limit  int
	window time.Duration
	now    func() time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{items: map[string]rateEntry{}, limit: limit, window: window, now: time.Now}
}

func (l *rateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	entry := l.items[key]
	if entry.start.IsZero() || now.Sub(entry.start) >= l.window {
		l.items[key] = rateEntry{count: 1, start: now}
		return true
	}
	entry.count++
	l.items[key] = entry
	// ponytail: process-local limiter; move counters to Redis when the API runs more than one replica.
	return entry.count <= l.limit
}
