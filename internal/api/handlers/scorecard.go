package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/scorecard"
)

type ScorecardHandler struct {
	pool *pgxpool.Pool
}

func NewScorecardHandler(pool *pgxpool.Pool) *ScorecardHandler {
	return &ScorecardHandler{pool: pool}
}

// Get handles GET /api/v1/agents/{id}/scorecard
func (h *ScorecardHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	var sc struct {
		ParticipantID  string          `json:"participant_id"`
		CompositeScore float64         `json:"composite_score"`
		Tier           string          `json:"tier"`
		Signals        json.RawMessage `json:"signals"`
		Weights        json.RawMessage `json:"weights"`
		ComputedAt     time.Time       `json:"computed_at"`
	}
	err := h.pool.QueryRow(r.Context(), `
		SELECT participant_id, composite_score, tier, signal_scores, weights, computed_at
		FROM agent_scorecards WHERE participant_id = $1`, id).Scan(
		&sc.ParticipantID, &sc.CompositeScore, &sc.Tier, &sc.Signals, &sc.Weights, &sc.ComputedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "scorecard not found")
			return
		}
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to load scorecard", err)
		return
	}

	api.JSON(w, http.StatusOK, sc)
}

// History handles GET /api/v1/agents/{id}/scorecard/history
func (h *ScorecardHandler) History(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	days := 90
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 && parsed <= 365 {
			days = parsed
		}
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT recorded_date, composite_score
		FROM scorecard_history
		WHERE participant_id = $1 AND recorded_date >= CURRENT_DATE - $2::int
		ORDER BY recorded_date ASC`, id, days)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to query history")
		return
	}
	defer rows.Close()

	type entry struct {
		Date  string  `json:"date"`
		Score float64 `json:"composite_score"`
	}
	var history []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.Date, &e.Score); err != nil {
			api.Error(w, http.StatusInternalServerError, "failed to scan history")
			return
		}
		history = append(history, e)
	}
	if history == nil {
		history = []entry{}
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"participant_id": id,
		"history":        history,
	})
}

// Weights handles GET /api/v1/scorecard/weights
func (h *ScorecardHandler) Weights(w http.ResponseWriter, r *http.Request) {
	api.JSON(w, http.StatusOK, map[string]any{
		"weights": scorecard.DefaultWeights(),
		"tiers": map[string]int{
			"elite":   85,
			"trusted": 65,
			"rising":  40,
			"new":     0,
		},
	})
}
