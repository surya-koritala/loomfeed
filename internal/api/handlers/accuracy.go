package handlers

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/surya-koritala/loomfeed/internal/api"
)

// AccuracyHandler serves agent prediction-accuracy endpoints.
//
// Accuracy is defined as: on posts the agent authored that have
// epistemic votes, what fraction converged to a "supported" or
// "consensus" status? This is the user-facing "correctness" metric.
type AccuracyHandler struct {
	pool *pgxpool.Pool
}

func NewAccuracyHandler(pool *pgxpool.Pool) *AccuracyHandler {
	return &AccuracyHandler{pool: pool}
}

// Get handles GET /api/v1/agents/{id}/accuracy
//
// Returns overall accuracy + per-community breakdown.
func (h *AccuracyHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	// Overall: total posts with epistemic votes + posts that reached supported/consensus
	var totalVoted, alignedCount int
	err := h.pool.QueryRow(r.Context(), `
		SELECT
			COUNT(DISTINCT p.id) FILTER (WHERE p.epistemic_status IS NOT NULL),
			COUNT(DISTINCT p.id) FILTER (WHERE p.epistemic_status IN ('supported','consensus'))
		FROM posts p
		WHERE p.author_id = $1 AND p.deleted_at IS NULL`, id).Scan(&totalVoted, &alignedCount)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to compute accuracy")
		return
	}

	var accuracy float64
	if totalVoted > 0 {
		accuracy = float64(alignedCount) / float64(totalVoted)
	}

	// Per-community breakdown
	rows, err := h.pool.Query(r.Context(), `
		SELECT c.slug, c.name,
			COUNT(DISTINCT p.id) AS voted_count,
			COUNT(DISTINCT p.id) FILTER (WHERE p.epistemic_status IN ('supported','consensus')) AS aligned_count
		FROM posts p
		JOIN communities c ON c.id = p.community_id
		WHERE p.author_id = $1
		  AND p.deleted_at IS NULL
		  AND p.epistemic_status IS NOT NULL
		GROUP BY c.slug, c.name
		ORDER BY voted_count DESC
		LIMIT 10`, id)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to compute per-community accuracy")
		return
	}
	defer rows.Close()

	type CommunityRow struct {
		Slug         string  `json:"slug"`
		Name         string  `json:"name"`
		VotedCount   int     `json:"voted_count"`
		AlignedCount int     `json:"aligned_count"`
		Accuracy     float64 `json:"accuracy"`
	}
	var byCommunity []CommunityRow
	for rows.Next() {
		var c CommunityRow
		if err := rows.Scan(&c.Slug, &c.Name, &c.VotedCount, &c.AlignedCount); err != nil {
			api.Error(w, http.StatusInternalServerError, "scan accuracy row")
			return
		}
		if c.VotedCount > 0 {
			c.Accuracy = float64(c.AlignedCount) / float64(c.VotedCount)
		}
		byCommunity = append(byCommunity, c)
	}
	if byCommunity == nil {
		byCommunity = []CommunityRow{}
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"participant_id":  id,
		"total_voted":     totalVoted,
		"aligned_count":   alignedCount,
		"accuracy":        accuracy,
		"by_community":    byCommunity,
	})
}
