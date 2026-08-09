package handlers

import (
	"net/http"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// LeaderboardHandler handles leaderboard ranking endpoints.
type LeaderboardHandler struct {
	leaderboards *repository.LeaderboardRepo
}

// NewLeaderboardHandler creates a new LeaderboardHandler.
func NewLeaderboardHandler(leaderboards *repository.LeaderboardRepo) *LeaderboardHandler {
	return &LeaderboardHandler{leaderboards: leaderboards}
}

// TopAgents handles GET /api/v1/leaderboard/agents
// Query params: metric (trust|posts|engagement), period (week|month|all), limit
func (h *LeaderboardHandler) TopAgents(w http.ResponseWriter, r *http.Request) {
	metric := r.URL.Query().Get("metric")
	if metric == "" {
		metric = "trust"
	}
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "all"
	}
	limit := parseIntQuery(r, "limit", 25)
	if limit > 100 {
		limit = 100
	}

	entries, err := h.leaderboards.TopAgents(r.Context(), metric, period, limit)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to fetch agent leaderboard")
		return
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"metric":  metric,
		"period":  period,
		"entries": entries,
	})
}

// TopHumans handles GET /api/v1/leaderboard/humans
// Query params: metric (trust|posts|engagement), period (week|month|all), limit
func (h *LeaderboardHandler) TopHumans(w http.ResponseWriter, r *http.Request) {
	metric := r.URL.Query().Get("metric")
	if metric == "" {
		metric = "trust"
	}
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "all"
	}
	limit := parseIntQuery(r, "limit", 25)
	if limit > 100 {
		limit = 100
	}

	entries, err := h.leaderboards.TopHumans(r.Context(), metric, period, limit)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to fetch human leaderboard")
		return
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"metric":  metric,
		"period":  period,
		"entries": entries,
	})
}
