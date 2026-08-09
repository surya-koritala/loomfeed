package handlers

import (
	"net/http"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// HeartbeatHandler handles agent heartbeat and online status endpoints.
type HeartbeatHandler struct {
	heartbeats *repository.HeartbeatRepo
}

// NewHeartbeatHandler creates a new HeartbeatHandler.
func NewHeartbeatHandler(heartbeats *repository.HeartbeatRepo) *HeartbeatHandler {
	return &HeartbeatHandler{heartbeats: heartbeats}
}

// Ping handles POST /api/v1/heartbeat — agent pings to signal online status.
// Requires authentication (JWT or API key).
func (h *HeartbeatHandler) Ping(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if err := h.heartbeats.Ping(r.Context(), claims.ParticipantID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to record heartbeat")
		return
	}

	// Log the heartbeat activity
	_, _ = h.heartbeats.Pool().Exec(r.Context(),
		`INSERT INTO agent_activity_log (participant_id, action_type) VALUES ($1, 'heartbeat')`,
		claims.ParticipantID)

	api.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ListOnline handles GET /api/v1/agents/online — returns currently online agents.
// Public endpoint.
func (h *HeartbeatHandler) ListOnline(w http.ResponseWriter, r *http.Request) {
	limit := parseIntQuery(r, "limit", 50)
	if limit > 200 {
		limit = 200
	}

	agents, err := h.heartbeats.ListOnline(r.Context(), limit)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list online agents")
		return
	}

	api.JSON(w, http.StatusOK, agents)
}

// OnlineCount handles GET /api/v1/agents/online/count — returns count of online agents.
// Public endpoint.
func (h *HeartbeatHandler) OnlineCount(w http.ResponseWriter, r *http.Request) {
	count, err := h.heartbeats.OnlineCount(r.Context())
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to get online count")
		return
	}

	api.JSON(w, http.StatusOK, map[string]int{"count": count})
}
