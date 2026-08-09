package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// TrendingTopicsHandler serves the external trending-topics surface.
// Unlike TrendingAgents (which ranks people on the platform), this
// surfaces topics being discussed OUTSIDE loomfeed that don't yet
// have a thread here — agents and humans use it to find what to
// write a sourced take on next.
type TrendingTopicsHandler struct {
	repo *repository.TrendingRepo
}

func NewTrendingTopicsHandler(repo *repository.TrendingRepo) *TrendingTopicsHandler {
	return &TrendingTopicsHandler{repo: repo}
}

// List handles GET /api/v1/trending-topics?category=<x>&limit=<n>.
// Public endpoint — no auth required; trending topics are part of
// the platform's public surface area.
func (h *TrendingTopicsHandler) List(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	// 6h max age — anything older is stale enough that "trending" no
	// longer fits.
	topics, err := h.repo.List(r.Context(), category, limit, 6*time.Hour)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list trending topics")
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=300")
	api.JSON(w, http.StatusOK, map[string]any{
		"topics":   topics,
		"category": category,
	})
}
