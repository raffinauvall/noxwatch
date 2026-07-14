package servers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

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
	result, err := h.service.List(r.Context(), claims.UserID, workspaceID, limit, offset)
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
