package enrollment

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/raffinauvall/noxwatch/apps/api/internal/auth"
	"github.com/raffinauvall/noxwatch/apps/api/internal/httpx"
)

var tagPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,31}$`)

type Handler struct {
	service *Service
	logger  *slog.Logger
}

func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, require func(http.Handler) http.Handler) {
	mux.Handle("POST /api/v1/servers/enrollment-token", require(http.HandlerFunc(h.createToken)))
	mux.Handle("GET /api/v1/enrollment-tokens/{tokenId}", require(http.HandlerFunc(h.tokenStatus)))
	mux.Handle("DELETE /api/v1/enrollment-tokens/{tokenId}", require(http.HandlerFunc(h.revokeToken)))
	mux.Handle("DELETE /api/v1/servers/{serverId}/agent", require(http.HandlerFunc(h.revokeAgent)))
	mux.HandleFunc("POST /api/v1/agent/enroll", h.enroll)
	mux.HandleFunc("POST /api/v1/agent/heartbeat", h.heartbeat)
}

func (h *Handler) createToken(w http.ResponseWriter, r *http.Request) {
	var input struct {
		WorkspaceID string   `json:"workspace_id"`
		Name        string   `json:"name"`
		Environment string   `json:"environment"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}
	if !httpx.Decode(w, r, &input) {
		return
	}
	fields := validateTokenInput(input.WorkspaceID, input.Name, input.Environment, input.Description, input.Tags)
	if len(fields) > 0 {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "The request contains invalid fields.", fields)
		return
	}
	input.Tags = uniqueTags(input.Tags)
	claims, _ := auth.Claims(r.Context())
	token, err := h.service.CreateToken(r.Context(), claims.UserID, input.WorkspaceID, strings.TrimSpace(input.Name), input.Environment, strings.TrimSpace(input.Description), input.Tags, clientIP(r))
	if errors.Is(err, ErrForbidden) {
		httpx.WriteError(w, r, http.StatusForbidden, "PERMISSION_DENIED", "You do not have permission to enroll servers in this workspace.", nil)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusCreated, token)
}

func (h *Handler) tokenStatus(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.Claims(r.Context())
	token, err := h.service.TokenStatus(r.Context(), claims.UserID, r.PathValue("tokenId"))
	if errors.Is(err, ErrEnrollmentNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "ENROLLMENT_NOT_FOUND", "Enrollment not found.", nil)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusOK, token)
}

func (h *Handler) revokeToken(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.Claims(r.Context())
	if err := h.service.RevokeToken(r.Context(), claims.UserID, r.PathValue("tokenId")); errors.Is(err, ErrEnrollmentNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "ENROLLMENT_NOT_FOUND", "Enrollment not found or already completed.", nil)
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusOK, map[string]bool{"revoked": true})
}

func (h *Handler) enroll(w http.ResponseWriter, r *http.Request) {
	var input EnrollInput
	if !httpx.Decode(w, r, &input) {
		return
	}
	if len(input.Hostname) < 1 || len(input.Hostname) > 253 || input.MachineID == "" || input.OS == "" || input.Architecture == "" || len(input.AgentVersion) > 50 {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Agent identity is incomplete or invalid.", nil)
		return
	}
	result, err := h.service.Enroll(r.Context(), input)
	if errors.Is(err, ErrTokenInvalid) {
		httpx.WriteError(w, r, http.StatusUnauthorized, "ENROLLMENT_TOKEN_INVALID", "The enrollment token is invalid, expired, or already used.", nil)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusCreated, result)
}

func (h *Handler) heartbeat(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ServerID string `json:"server_id"`
	}
	if !httpx.Decode(w, r, &input) {
		return
	}
	credential := bearer(r)
	if credential == "" || input.ServerID == "" {
		httpx.WriteError(w, r, http.StatusUnauthorized, "AGENT_AUTHENTICATION_FAILED", "Agent authentication failed.", nil)
		return
	}
	if err := h.service.Heartbeat(r.Context(), credential, input.ServerID); errors.Is(err, ErrAgentUnauthorized) {
		httpx.WriteError(w, r, http.StatusUnauthorized, "AGENT_AUTHENTICATION_FAILED", "Agent authentication failed.", nil)
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusOK, map[string]int{"next_heartbeat_seconds": 20})
}

func (h *Handler) revokeAgent(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.Claims(r.Context())
	if err := h.service.RevokeAgent(r.Context(), claims.UserID, r.PathValue("serverId"), clientIP(r)); errors.Is(err, ErrForbidden) {
		httpx.WriteError(w, r, http.StatusNotFound, "SERVER_NOT_FOUND", "Server not found.", nil)
		return
	} else if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusOK, map[string]bool{"revoked": true})
}

func validateTokenInput(workspaceID, name, environment, description string, tags []string) map[string]string {
	fields := map[string]string{}
	if workspaceID == "" {
		fields["workspace_id"] = "Workspace is required."
	}
	if length := len(strings.TrimSpace(name)); length < 1 || length > 100 {
		fields["name"] = "Server name must be between 1 and 100 characters."
	}
	if !slices.Contains([]string{"production", "staging", "development", "testing", "other"}, environment) {
		fields["environment"] = "Choose a valid environment."
	}
	if len(strings.TrimSpace(description)) > 500 {
		fields["description"] = "Description must not exceed 500 characters."
	}
	if len(tags) > 20 {
		fields["tags"] = "Use at most 20 tags."
	}
	for _, tag := range tags {
		if !tagPattern.MatchString(tag) {
			fields["tags"] = "Tags may contain letters, numbers, dots, underscores, colons, and dashes."
			break
		}
	}
	return fields
}

func uniqueTags(tags []string) []string {
	result := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, tag := range tags {
		tag = strings.ToLower(tag)
		if !seen[tag] {
			seen[tag] = true
			result = append(result, tag)
		}
	}
	return result
}

func bearer(r *http.Request) string {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return ""
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return ""
}

func (h *Handler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("enrollment request failed", "request_id", r.Header.Get("X-Request-ID"), "error", err)
	httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred.", nil)
}
