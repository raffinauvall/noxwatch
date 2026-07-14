package metrics

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/raffinauvall/noxwatch/apps/api/internal/auth"
	"github.com/raffinauvall/noxwatch/apps/api/internal/enrollment"
	"github.com/raffinauvall/noxwatch/apps/api/internal/httpx"
)

type Handler struct {
	service *Service
	logger  *slog.Logger
	mu      sync.Mutex
	rates   map[string]rate
}

type rate struct {
	minute int64
	count  int
}

func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger, rates: map[string]rate{}}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux, require func(http.Handler) http.Handler) {
	mux.HandleFunc("POST /api/v1/agent/metrics", h.ingest)
	mux.Handle("GET /api/v1/servers/{serverId}/metrics", require(http.HandlerFunc(h.history)))
	mux.Handle("GET /api/v1/servers/{serverId}/metrics/latest", require(http.HandlerFunc(h.latest)))
}

func (h *Handler) ingest(w http.ResponseWriter, r *http.Request) {
	credential := bearer(r)
	if credential == "" {
		httpx.WriteError(w, r, http.StatusUnauthorized, "AGENT_AUTHENTICATION_FAILED", "Agent authentication failed.", nil)
		return
	}
	if !h.allow(credential) {
		httpx.WriteError(w, r, http.StatusTooManyRequests, "AGENT_RATE_LIMITED", "Metrics rate limit exceeded.", nil)
		return
	}
	var payload Payload
	if !httpx.Decode(w, r, &payload) {
		return
	}
	if fields := validate(payload, time.Now().UTC()); len(fields) > 0 {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "METRICS_INVALID", "The metrics payload is invalid.", fields)
		return
	}
	duplicate, err := h.service.Ingest(r.Context(), credential, payload)
	if errors.Is(err, enrollment.ErrAgentUnauthorized) {
		httpx.WriteError(w, r, http.StatusUnauthorized, "AGENT_AUTHENTICATION_FAILED", "Agent authentication failed.", nil)
		return
	}
	if err != nil {
		h.logger.Error("metrics ingestion failed", "request_id", r.Header.Get("X-Request-ID"), "server_id", payload.ServerID, "error", err)
		httpx.WriteError(w, r, http.StatusInternalServerError, "INGESTION_FAILED", "Metrics could not be accepted.", nil)
		return
	}
	httpx.Write(w, http.StatusAccepted, map[string]bool{"accepted": true, "duplicate": duplicate})
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	from, err := parseTime(r.URL.Query().Get("from"), now.Add(-time.Hour))
	if err != nil {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "from must be an RFC3339 timestamp.", nil)
		return
	}
	to, err := parseTime(r.URL.Query().Get("to"), now)
	if err != nil || !to.After(from) || to.Sub(from) > 30*24*time.Hour {
		httpx.WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Metrics range must be positive and no longer than 30 days.", nil)
		return
	}
	limit := 2000
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 2000 {
			httpx.WriteError(w, r, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "limit must be between 1 and 2000.", nil)
			return
		}
		limit = value
	}
	claims, _ := auth.Claims(r.Context())
	samples, err := h.service.History(r.Context(), claims.UserID, r.PathValue("serverId"), from, to, limit)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusOK, samples)
}

func (h *Handler) latest(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.Claims(r.Context())
	sample, err := h.service.Latest(r.Context(), claims.UserID, r.PathValue("serverId"))
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "METRICS_NOT_FOUND", "No metrics are available for this server.", nil)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	httpx.Write(w, http.StatusOK, sample)
}

func validate(payload Payload, now time.Time) map[string]string {
	fields := map[string]string{}
	if payload.ServerID == "" || payload.Sequence <= 0 {
		fields["identity"] = "server_id and a positive sequence are required."
	}
	if payload.CollectedAt.IsZero() || payload.CollectedAt.After(now.Add(2*time.Minute)) || payload.CollectedAt.Before(now.Add(-24*time.Hour)) {
		fields["collected_at"] = "Timestamp must be within the accepted 24-hour window."
	}
	percentages := []float64{payload.CPU.UsagePercent, payload.Memory.UsagePercent, payload.Swap.UsagePercent}
	for _, disk := range payload.Disks {
		percentages = append(percentages, disk.UsagePercent, disk.InodeUsagePercent)
		if disk.MountPoint == "" || disk.Filesystem == "" || disk.TotalBytes < 0 || disk.UsedBytes < 0 || disk.AvailableBytes < 0 {
			fields["disks"] = "Disk metrics contain invalid values."
		}
	}
	for _, value := range percentages {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 100 {
			fields["percentages"] = "Percentages must be between 0 and 100."
			break
		}
	}
	if payload.System.UptimeSeconds < 0 || payload.System.ProcessCount < 0 || payload.Memory.TotalBytes < 0 || payload.Memory.UsedBytes < 0 || payload.Memory.AvailableBytes < 0 || payload.Swap.TotalBytes < 0 || payload.Swap.UsedBytes < 0 {
		fields["values"] = "Metric values cannot be negative."
	}
	if len(payload.Disks) > 100 || len(payload.Networks) > 100 {
		fields["collections"] = "At most 100 disk and network entries are accepted."
	}
	for _, network := range payload.Networks {
		if network.Interface == "" || network.RXBytesTotal < 0 || network.TXBytesTotal < 0 || network.RXBytesPerSecond < 0 || network.TXBytesPerSecond < 0 {
			fields["networks"] = "Network metrics contain invalid values."
			break
		}
	}
	return fields
}

func (h *Handler) allow(credential string) bool {
	sum := sha256.Sum256([]byte(credential))
	key := hex.EncodeToString(sum[:])
	minute := time.Now().Unix() / 60
	h.mu.Lock()
	defer h.mu.Unlock()
	entry := h.rates[key]
	if entry.minute != minute {
		entry = rate{minute: minute}
	}
	entry.count++
	h.rates[key] = entry
	// ponytail: process-local ingestion limits; move counters to Redis with multiple API replicas.
	return entry.count <= 120
}

func bearer(r *http.Request) string {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return ""
}

func parseTime(raw string, fallback time.Time) (time.Time, error) {
	if raw == "" {
		return fallback, nil
	}
	return time.Parse(time.RFC3339, raw)
}

func (h *Handler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("metrics query failed", "request_id", r.Header.Get("X-Request-ID"), "error", err)
	httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred.", nil)
}
