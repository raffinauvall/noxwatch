package notifications

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/raffinauvall/noxwatch/apps/api/internal/auth"
	"github.com/raffinauvall/noxwatch/apps/api/internal/config"
	"github.com/raffinauvall/noxwatch/apps/api/internal/httpx"
)

type Handler struct {
	service *Service
	logger  *slog.Logger
	cfg     config.Config
}

func NewHandler(service *Service, cfg config.Config, logger *slog.Logger) *Handler {
	return &Handler{service: service, cfg: cfg, logger: logger}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, require func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/notification-channels", require(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/v1/notification-channels", require(http.HandlerFunc(h.create)))
	mux.Handle("DELETE /api/v1/notification-channels/{channelId}", require(http.HandlerFunc(h.delete)))
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		WorkspaceID string `json:"workspace_id"`
		Name        string `json:"name"`
		URL         string `json:"url"`
	}
	if !httpx.Decode(w, r, &input) {
		return
	}
	if fields := h.validate(input.WorkspaceID, input.Name, input.URL); len(fields) > 0 {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "The request contains invalid fields.", fields)
		return
	}
	claims, _ := auth.Claims(r.Context())
	channel, err := h.service.Create(r.Context(), claims.UserID, input.WorkspaceID, strings.TrimSpace(input.Name), input.URL, clientIP(r))
	if errors.Is(err, ErrForbidden) {
		httpx.WriteError(w, r, http.StatusForbidden, "PERMISSION_DENIED", "You do not have permission to manage integrations.", nil)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusCreated, channel)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "workspace_id is required.", nil)
		return
	}
	claims, _ := auth.Claims(r.Context())
	channels, err := h.service.List(r.Context(), claims.UserID, workspaceID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusOK, channels)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.Claims(r.Context())
	if err := h.service.Delete(r.Context(), claims.UserID, r.PathValue("channelId")); errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "CHANNEL_NOT_FOUND", "Notification channel not found.", nil)
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (h *Handler) validate(workspaceID, name, rawURL string) map[string]string {
	fields := map[string]string{}
	if workspaceID == "" {
		fields["workspace_id"] = "Workspace is required."
	}
	if length := len(strings.TrimSpace(name)); length < 1 || length > 100 {
		fields["name"] = "Name must be between 1 and 100 characters."
	}
	parsed, err := url.Parse(rawURL)
	allowedScheme := parsed.Scheme == "https" || (h.cfg.AppEnv == "development" && parsed.Scheme == "http")
	if err != nil || parsed.Host == "" || parsed.User != nil || !allowedScheme || len(rawURL) > 2000 {
		fields["url"] = "Enter a valid HTTPS webhook URL."
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

func (h *Handler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("notification request failed", "request_id", r.Header.Get("X-Request-ID"), "error", err)
	httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred.", nil)
}
