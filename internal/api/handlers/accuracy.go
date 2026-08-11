package handlers

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/surya-koritala/loomfeed/internal/api"
)

// AccuracyHandler serves participant prediction-accuracy endpoints.
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

	var totalResolved, correctCount int
	var calibratedAccuracy float64
	err := h.pool.QueryRow(r.Context(), `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE outcome = 'correct'),
		       COALESCE(AVG(
		           CASE
		               WHEN brier IS NOT NULL THEN
		                   1 - LEAST(1, GREATEST(0,
		                       brier::float8 / CASE WHEN match_id IS NOT NULL THEN 2.0 ELSE 1.0 END
		                   ))
		               WHEN outcome = 'correct' THEN 1.0
		               ELSE 0.0
		           END
		       ), 0)
		FROM predictions
		WHERE participant_id = $1 AND outcome IS NOT NULL`, id).Scan(&totalResolved, &correctCount, &calibratedAccuracy)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to compute accuracy")
		return
	}

	var accuracy float64
	if totalResolved > 0 {
		accuracy = float64(correctCount) / float64(totalResolved)
	}

	// Per-community breakdown applies to post-attached predictions. Sports
	// forecasts are included in the overall totals but have no community.
	rows, err := h.pool.Query(r.Context(), `
		SELECT c.slug, c.name,
			COUNT(*) AS resolved_count,
			COUNT(*) FILTER (WHERE pred.outcome = 'correct') AS correct_count,
			COALESCE(AVG(1 - LEAST(1, GREATEST(0, pred.brier::float8))), 0)
		FROM predictions pred
		JOIN posts p ON p.id = pred.post_id
		JOIN communities c ON c.id = p.community_id
		WHERE pred.participant_id = $1
		  AND pred.outcome IS NOT NULL
		  AND p.deleted_at IS NULL
		GROUP BY c.slug, c.name
		ORDER BY resolved_count DESC
		LIMIT 10`, id)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to compute per-community accuracy")
		return
	}
	defer rows.Close()

	type CommunityRow struct {
		Slug               string  `json:"slug"`
		Name               string  `json:"name"`
		ResolvedCount      int     `json:"resolved_count"`
		CorrectCount       int     `json:"correct_count"`
		Accuracy           float64 `json:"accuracy"`
		CalibratedAccuracy float64 `json:"calibrated_accuracy"`
	}
	var byCommunity []CommunityRow
	for rows.Next() {
		var c CommunityRow
		if err := rows.Scan(&c.Slug, &c.Name, &c.ResolvedCount, &c.CorrectCount, &c.CalibratedAccuracy); err != nil {
			api.Error(w, http.StatusInternalServerError, "scan accuracy row")
			return
		}
		if c.ResolvedCount > 0 {
			c.Accuracy = float64(c.CorrectCount) / float64(c.ResolvedCount)
		}
		byCommunity = append(byCommunity, c)
	}
	if byCommunity == nil {
		byCommunity = []CommunityRow{}
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"participant_id":      id,
		"total_resolved":      totalResolved,
		"correct_count":       correctCount,
		"accuracy":            accuracy,
		"calibrated_accuracy": calibratedAccuracy,
		// Compatibility aliases for clients deployed before generic predictions.
		"total_voted":   totalResolved,
		"aligned_count": correctCount,
		"by_community":  byCommunity,
	})
}
