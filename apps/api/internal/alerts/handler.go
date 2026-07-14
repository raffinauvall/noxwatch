package alerts

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/raffinauvall/noxwatch/apps/api/internal/auth"
	"github.com/raffinauvall/noxwatch/apps/api/internal/httpx"
)

type Handler struct {
	service *Service
	logger  *slog.Logger
}

type ruleInput struct {
	WorkspaceID       string   `json:"workspace_id"`
	ServerID          string   `json:"server_id"`
	Name              string   `json:"name"`
	Metric            string   `json:"metric"`
	WarningThreshold  *float64 `json:"warning_threshold"`
	CriticalThreshold *float64 `json:"critical_threshold"`
	EvaluationSeconds int      `json:"evaluation_seconds"`
	CooldownSeconds   int      `json:"cooldown_seconds"`
}

func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, require func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/alert-rules", require(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/v1/alert-rules", require(http.HandlerFunc(h.create)))
	mux.Handle("PATCH /api/v1/alert-rules/{ruleId}", require(http.HandlerFunc(h.update)))
	mux.Handle("DELETE /api/v1/alert-rules/{ruleId}", require(http.HandlerFunc(h.delete)))
	mux.Handle("GET /api/v1/servers/{serverId}/alerts", require(http.HandlerFunc(h.events)))
	mux.Handle("GET /api/v1/alerts", require(http.HandlerFunc(h.workspaceEvents)))
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var input ruleInput
	if !httpx.Decode(w, r, &input) {
		return
	}
	if fields := validateRule(input); len(fields) > 0 {
		h.validationError(w, r, fields)
		return
	}
	claims, _ := auth.Claims(r.Context())
	rule, err := h.service.Create(r.Context(), claims.UserID, input.WorkspaceID, input.ServerID, strings.TrimSpace(input.Name), input.Metric, input.WarningThreshold, input.CriticalThreshold, input.EvaluationSeconds, input.CooldownSeconds, clientIP(r))
	if errors.Is(err, ErrForbidden) {
		h.forbidden(w, r)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusCreated, rule)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		h.validationError(w, r, map[string]string{"workspace_id": "Workspace is required."})
		return
	}
	claims, _ := auth.Claims(r.Context())
	rules, err := h.service.List(r.Context(), claims.UserID, workspaceID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusOK, rules)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name              *string  `json:"name"`
		WarningThreshold  *float64 `json:"warning_threshold"`
		CriticalThreshold *float64 `json:"critical_threshold"`
		EvaluationSeconds *int     `json:"evaluation_seconds"`
		CooldownSeconds   *int     `json:"cooldown_seconds"`
		Enabled           *bool    `json:"enabled"`
	}
	if !httpx.Decode(w, r, &input) {
		return
	}
	if fields := validateUpdate(input.Name, input.WarningThreshold, input.CriticalThreshold, input.EvaluationSeconds, input.CooldownSeconds); len(fields) > 0 {
		h.validationError(w, r, fields)
		return
	}
	if input.Name != nil {
		trimmed := strings.TrimSpace(*input.Name)
		input.Name = &trimmed
	}
	claims, _ := auth.Claims(r.Context())
	rule, err := h.service.Update(r.Context(), claims.UserID, r.PathValue("ruleId"), input.Name, input.WarningThreshold, input.CriticalThreshold, input.EvaluationSeconds, input.CooldownSeconds, input.Enabled)
	if errors.Is(err, ErrNotFound) {
		h.notFound(w, r)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusOK, rule)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.Claims(r.Context())
	if err := h.service.Delete(r.Context(), claims.UserID, r.PathValue("ruleId")); errors.Is(err, ErrNotFound) {
		h.notFound(w, r)
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.Claims(r.Context())
	events, err := h.service.Events(r.Context(), claims.UserID, r.PathValue("serverId"))
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusOK, events)
}

func (h *Handler) workspaceEvents(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.URL.Query().Get("workspace_id")
	if workspaceID == "" {
		h.validationError(w, r, map[string]string{"workspace_id": "Workspace is required."})
		return
	}
	claims, _ := auth.Claims(r.Context())
	events, err := h.service.WorkspaceEvents(r.Context(), claims.UserID, workspaceID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusOK, events)
}

func validateRule(input ruleInput) map[string]string {
	fields := validateUpdate(&input.Name, input.WarningThreshold, input.CriticalThreshold, &input.EvaluationSeconds, &input.CooldownSeconds)
	if input.WorkspaceID == "" {
		fields["workspace_id"] = "Workspace is required."
	}
	if input.ServerID == "" {
		fields["server_id"] = "Server is required."
	}
	metrics := map[string]bool{"cpu_usage": true, "memory_usage": true, "disk_usage": true, "swap_usage": true, "server_offline": true, "agent_disconnected": true}
	if !metrics[input.Metric] {
		fields["metric"] = "Select a supported metric."
	}
	connectivity := input.Metric == "server_offline" || input.Metric == "agent_disconnected"
	if !connectivity && (input.WarningThreshold == nil || input.CriticalThreshold == nil) {
		fields["thresholds"] = "Warning and critical thresholds are required."
	}
	if input.WarningThreshold != nil && input.CriticalThreshold != nil && *input.CriticalThreshold < *input.WarningThreshold {
		fields["critical_threshold"] = "Critical threshold must be at least the warning threshold."
	}
	return fields
}

func validateUpdate(name *string, warning, critical *float64, duration, cooldown *int) map[string]string {
	fields := map[string]string{}
	if name != nil && (len(strings.TrimSpace(*name)) < 1 || len(strings.TrimSpace(*name)) > 120) {
		fields["name"] = "Name must be between 1 and 120 characters."
	}
	for key, value := range map[string]*float64{"warning_threshold": warning, "critical_threshold": critical} {
		if value != nil && (*value < 0 || *value > 100) {
			fields[key] = "Threshold must be between 0 and 100."
		}
	}
	if duration != nil && (*duration < 0 || *duration > 86400) {
		fields["evaluation_seconds"] = "Evaluation duration must be between 0 and 86400 seconds."
	}
	if cooldown != nil && (*cooldown < 0 || *cooldown > 604800) {
		fields["cooldown_seconds"] = "Cooldown must be between 0 and 604800 seconds."
	}
	return fields
}

func (h *Handler) validationError(w http.ResponseWriter, r *http.Request, fields map[string]string) {
	httpx.WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "The request contains invalid fields.", fields)
}

func (h *Handler) forbidden(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, r, http.StatusForbidden, "PERMISSION_DENIED", "You do not have permission to manage alert rules.", nil)
}

func (h *Handler) notFound(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, r, http.StatusNotFound, "ALERT_RULE_NOT_FOUND", "Alert rule not found.", nil)
}

func (h *Handler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("alert request failed", "request_id", r.Header.Get("X-Request-ID"), "error", err)
	httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred.", nil)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return ""
}
