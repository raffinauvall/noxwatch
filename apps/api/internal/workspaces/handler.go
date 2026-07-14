package workspaces

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

func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, require func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/workspaces", require(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/v1/workspaces", require(http.HandlerFunc(h.create)))
	mux.Handle("GET /api/v1/workspaces/{workspaceId}", require(http.HandlerFunc(h.get)))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.Claims(r.Context())
	workspaces, err := h.service.List(r.Context(), claims.UserID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusOK, workspaces)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
	}
	if !httpx.Decode(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if len(input.Name) < 2 || len(input.Name) > 100 {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "The request contains invalid fields.", map[string]string{"name": "Workspace name must be between 2 and 100 characters."})
		return
	}
	claims, _ := auth.Claims(r.Context())
	workspace, err := h.service.Create(r.Context(), claims.UserID, input.Name, clientIP(r))
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusCreated, workspace)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.Claims(r.Context())
	workspace, err := h.service.Get(r.Context(), claims.UserID, r.PathValue("workspaceId"))
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "WORKSPACE_NOT_FOUND", "Workspace not found.", nil)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusOK, workspace)
}

func (h *Handler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("workspace request failed", "request_id", r.Header.Get("X-Request-ID"), "error", err)
	httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred.", nil)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return ""
}
