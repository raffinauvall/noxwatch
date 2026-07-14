package servers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/raffinauvall/noxwatch/apps/api/internal/auth"
	"github.com/raffinauvall/noxwatch/apps/api/internal/httpx"
)

type Handler struct {
	service *Service
	logger  *slog.Logger
}

func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, require func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/servers", require(http.HandlerFunc(h.list)))
	mux.Handle("GET /api/v1/servers/{serverId}", require(http.HandlerFunc(h.get)))
	mux.Handle("PATCH /api/v1/servers/{serverId}", require(http.HandlerFunc(h.update)))
	mux.Handle("DELETE /api/v1/servers/{serverId}", require(http.HandlerFunc(h.delete)))
	mux.Handle("GET /api/v1/workspaces/{workspaceId}/events", require(http.HandlerFunc(h.events)))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        *string   `json:"name"`
		Description *string   `json:"description"`
		Environment *string   `json:"environment"`
		Tags        *[]string `json:"tags"`
		Maintenance *bool     `json:"maintenance"`
	}
	if !httpx.Decode(w, r, &input) {
		return
	}
	if fields := validateUpdate(input.Name, input.Description, input.Environment, input.Tags); len(fields) > 0 {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "The request contains invalid fields.", fields)
		return
	}
	if input.Name != nil {
		value := strings.TrimSpace(*input.Name)
		input.Name = &value
	}
	if input.Description != nil {
		value := strings.TrimSpace(*input.Description)
		input.Description = &value
	}
	if input.Tags != nil {
		tags := normalizeTags(*input.Tags)
		input.Tags = &tags
	}
	claims, _ := auth.Claims(r.Context())
	server, err := h.service.Update(r.Context(), claims.UserID, r.PathValue("serverId"), UpdateInput{Name: input.Name, Description: input.Description, Environment: input.Environment, Tags: input.Tags, Maintenance: input.Maintenance}, clientIP(r))
	if errors.Is(err, ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusOK, server)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.Claims(r.Context())
	if err := h.service.Delete(r.Context(), claims.UserID, r.PathValue("serverId"), clientIP(r)); errors.Is(err, ErrNotFound) {
		h.notFound(w, r)
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.WriteError(w, r, http.StatusInternalServerError, "STREAM_UNAVAILABLE", "Live status streaming is unavailable.", nil)
		return
	}
	claims, _ := auth.Claims(r.Context())
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	expires := time.NewTimer(time.Until(claims.ExpiresAt))
	defer expires.Stop()
	for {
		statuses, err := h.service.Statuses(r.Context(), claims.UserID, r.PathValue("workspaceId"))
		if err != nil {
			h.logger.Error("status stream failed", "request_id", r.Header.Get("X-Request-ID"), "error", err)
			return
		}
		body, _ := json.Marshal(statuses)
		_, _ = w.Write([]byte("event: server-status\ndata: "))
		_, _ = w.Write(body)
		_, _ = w.Write([]byte("\n\n"))
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-expires.C:
			return
		case <-ticker.C:
		}
	}
}

func validateUpdate(name, description, environment *string, tags *[]string) map[string]string {
	fields := map[string]string{}
	if name != nil && (len(strings.TrimSpace(*name)) < 1 || len(strings.TrimSpace(*name)) > 100) {
		fields["name"] = "Server name must be between 1 and 100 characters."
	}
	if description != nil && len(strings.TrimSpace(*description)) > 500 {
		fields["description"] = "Description must not exceed 500 characters."
	}
	if environment != nil && !slices.Contains([]string{"production", "staging", "development", "testing", "other"}, *environment) {
		fields["environment"] = "Choose a valid environment."
	}
	if tags != nil {
		if len(*tags) > 20 {
			fields["tags"] = "Use at most 20 tags."
		}
		for _, tag := range *tags {
			tag = strings.TrimSpace(tag)
			if len(tag) < 1 || len(tag) > 32 || strings.ContainsAny(tag, " \\/'\"") {
				fields["tags"] = "Tags contain unsupported characters."
				break
			}
		}
	}
	return fields
}

func normalizeTags(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return ""
}

func (h *Handler) notFound(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, r, http.StatusNotFound, "SERVER_NOT_FOUND", "Server not found.", nil)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "workspace_id is required.", nil)
		return
	}
	limit := boundedInt(r.URL.Query().Get("limit"), 50, 1, 100)
	offset := boundedInt(r.URL.Query().Get("offset"), 0, 0, 100_000)
	claims, _ := auth.Claims(r.Context())
	status := r.URL.Query().Get("status")
	environment := r.URL.Query().Get("environment")
	sort := r.URL.Query().Get("sort")
	if status != "" && !slices.Contains([]string{"online", "degraded", "warning", "offline", "unknown", "maintenance"}, status) {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "status is invalid.", nil)
		return
	}
	if environment != "" && !slices.Contains([]string{"production", "staging", "development", "testing", "other"}, environment) {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "environment is invalid.", nil)
		return
	}
	if sort != "" && !slices.Contains([]string{"name", "status", "recent"}, sort) {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "sort is invalid.", nil)
		return
	}
	search, tag := strings.TrimSpace(r.URL.Query().Get("search")), strings.ToLower(strings.TrimSpace(r.URL.Query().Get("tag")))
	if len(search) > 100 || len(tag) > 32 {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "search or tag is too long.", nil)
		return
	}
	result, err := h.service.List(r.Context(), claims.UserID, workspaceID, ListOptions{Limit: limit, Offset: offset, Search: search, Status: status, Environment: environment, Tag: tag, Sort: sort})
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusOK, result)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.Claims(r.Context())
	result, err := h.service.Get(r.Context(), claims.UserID, r.PathValue("serverId"))
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "SERVER_NOT_FOUND", "Server not found.", nil)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusOK, result)
}

func boundedInt(raw string, fallback, min, max int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value < min || value > max {
		return fallback
	}
	return value
}

func (h *Handler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("server request failed", "request_id", r.Header.Get("X-Request-ID"), "error", err)
	httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred.", nil)
}
