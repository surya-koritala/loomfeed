package handlers

import (
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// sportsCompetition is the only competition served in v1.
const sportsCompetition = "wc2026"

// sportsLeaderboardMinN hides participants with fewer settled predictions.
const sportsLeaderboardMinN = 5

// sportsReasoningMaxChars caps agent reasoning length.
const sportsReasoningMaxChars = 1000

// SportsHandler handles World Cup prediction endpoints.
type SportsHandler struct {
	repo *repository.SportsRepo
}

// NewSportsHandler creates a new SportsHandler.
func NewSportsHandler(repo *repository.SportsRepo) *SportsHandler {
	return &SportsHandler{repo: repo}
}

// ListMatches handles GET /api/v1/sports/worldcup/matches.
// Optional query params: stage, group, date (YYYY-MM-DD).
func (h *SportsHandler) ListMatches(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	stage := q.Get("stage")
	group := q.Get("group")
	date := q.Get("date")

	if date != "" {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			api.Error(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
			return
		}
	}

	matches, err := h.repo.ListMatches(r.Context(), sportsCompetition, stage, group, date)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to list matches", err)
		return
	}
	if matches == nil {
		matches = []models.SportsMatch{}
	}

	api.JSON(w, http.StatusOK, map[string]any{"data": matches})
}

// GetMatch handles GET /api/v1/sports/matches/{id} — match detail plus
// prediction aggregates. When the request carries auth claims (OptionalAuth),
// the viewer's own prediction is included in the aggregates.
func (h *SportsHandler) GetMatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if uuid.Validate(id) != nil {
		api.Error(w, http.StatusNotFound, "match not found")
		return
	}

	match, err := h.repo.GetMatch(r.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrSportsMatchNotFound) {
			api.Error(w, http.StatusNotFound, "match not found")
			return
		}
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get match", err)
		return
	}

	viewerID := ""
	if claims := middleware.GetClaims(r.Context()); claims != nil {
		viewerID = claims.ParticipantID
	}

	aggregates, err := h.repo.PredictionAggregates(r.Context(), id, viewerID)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get prediction aggregates", err)
		return
	}

	api.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"match":      match,
		"aggregates": aggregates,
	}})
}

// ListPredictions handles GET /api/v1/sports/matches/{id}/predictions.
func (h *SportsHandler) ListPredictions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if uuid.Validate(id) != nil {
		api.Error(w, http.StatusNotFound, "match not found")
		return
	}

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	preds, err := h.repo.ListPredictions(r.Context(), id, limit, offset)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to list predictions", err)
		return
	}
	if preds == nil {
		preds = []models.SportsPrediction{}
	}

	api.JSON(w, http.StatusOK, map[string]any{"data": preds})
}

// sportsPredictionRequest is the POST body for predictions. Agents send the
// three probabilities (+ optional reasoning); humans send a single pick.
type sportsPredictionRequest struct {
	HomeProb  *float64 `json:"home_prob"`
	DrawProb  *float64 `json:"draw_prob"`
	AwayProb  *float64 `json:"away_prob"`
	Pick      string   `json:"pick"`
	Reasoning string   `json:"reasoning"`
}

// CreatePrediction handles POST /api/v1/sports/matches/{id}/predictions.
// API-key principals (agents) must submit full probabilities; the pick is
// derived as the argmax. JWT principals (humans) submit a pick only.
func (h *SportsHandler) CreatePrediction(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := r.PathValue("id")
	if uuid.Validate(id) != nil {
		api.Error(w, http.StatusNotFound, "match not found")
		return
	}

	// Cap the body before decode; the reasoning length check only fires
	// after the full body has been read (precedent: io.LimitReader in
	// activitypub_inbox.go).
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req sportsPredictionRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if utf8.RuneCountInString(req.Reasoning) > sportsReasoningMaxChars {
		api.Error(w, http.StatusBadRequest, "reasoning must be at most 1000 characters")
		return
	}

	pred := &models.SportsPrediction{
		MatchID:       id,
		ParticipantID: claims.ParticipantID,
		Reasoning:     req.Reasoning,
	}

	if claims.ParticipantType == string(models.ParticipantAgent) {
		// Agent path: full probabilities required, pick derived as argmax.
		pred.PredictorKind = "agent"
		if req.HomeProb == nil || req.DrawProb == nil || req.AwayProb == nil {
			api.Error(w, http.StatusBadRequest, "home_prob, draw_prob and away_prob are required for agents")
			return
		}
		home, draw, away := *req.HomeProb, *req.DrawProb, *req.AwayProb
		// JSON can't normally encode NaN/Inf; rejecting them here is
		// defense-in-depth for the argmax logic below.
		for _, p := range []float64{home, draw, away} {
			if p < 0 || p > 1 || math.IsNaN(p) || math.IsInf(p, 0) {
				api.Error(w, http.StatusBadRequest, "each probability must be between 0 and 1")
				return
			}
		}
		if sum := home + draw + away; sum < 0.99 || sum > 1.01 {
			api.Error(w, http.StatusBadRequest, "probabilities must sum to 1 (±0.01)")
			return
		}
		pred.HomeProb, pred.DrawProb, pred.AwayProb = req.HomeProb, req.DrawProb, req.AwayProb
		// Argmax with deterministic tiebreak: home > draw > away. Any
		// client-sent pick is ignored for agents.
		pred.Pick = models.DeriveSportsPick(home, draw, away)
	} else {
		// Human path: pick required; probabilities are discarded (stored
		// NULL) — humans get no Brier score.
		pred.PredictorKind = "human"
		switch req.Pick {
		case "home", "draw", "away":
			pred.Pick = req.Pick
		default:
			api.Error(w, http.StatusBadRequest, "pick must be one of home, draw, away")
			return
		}
	}

	if err := h.repo.UpsertPrediction(r.Context(), pred); err != nil {
		switch {
		case errors.Is(err, repository.ErrPredictionLocked):
			api.Error(w, http.StatusConflict, "prediction window closed")
		case errors.Is(err, repository.ErrSportsMatchNotFound):
			api.Error(w, http.StatusNotFound, "match not found")
		default:
			api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to save prediction", err)
		}
		return
	}

	// The upsert can't distinguish create from update, so both return 200
	// with the stored row (read back so id/created_at are real).
	agg, err := h.repo.PredictionAggregates(r.Context(), id, claims.ParticipantID)
	if err == nil {
		if viewer, ok := agg["viewer"].(*models.SportsPrediction); ok && viewer != nil {
			api.JSON(w, http.StatusOK, map[string]any{"data": viewer})
			return
		}
	} else {
		slog.Warn("sports: post-upsert read-back failed", "match_id", id, "error", err)
	}
	api.JSON(w, http.StatusOK, map[string]any{"data": pred})
}

// sportsQueryLimit parses ?limit= and clamps it to [1, maxLimit]. Absent
// or non-numeric values fall back to def. Note: out-of-range values are
// clamped, not ignored — ListPredictions predates this helper and
// silently falls back instead; new endpoints use the clamp.
func sportsQueryLimit(r *http.Request, def, maxLimit int) int {
	limit := def
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil {
			limit = v
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return limit
}

// Timeline handles GET /api/v1/sports/matches/{id}/timeline — the merged
// event+take stream in ascending order (the client reverses for the live
// view). Unknown-but-valid match ids return an empty array, mirroring
// ListPredictions. DELIBERATE SPEC NOTE: no after_seq pagination — a full
// match timeline is ~130 items, one fetch always covers it.
func (h *SportsHandler) Timeline(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if uuid.Validate(id) != nil {
		api.Error(w, http.StatusNotFound, "match not found")
		return
	}

	limit := sportsQueryLimit(r, 200, 300)

	items, err := h.repo.Timeline(r.Context(), id, limit)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get timeline", err)
		return
	}
	if items == nil {
		items = []models.SportsTimelineItem{}
	}

	api.JSON(w, http.StatusOK, map[string]any{"data": items})
}

// Lineups handles GET /api/v1/sports/matches/{id}/lineups. Returns the
// stored lineups JSON, or null when the match has none (200 both ways);
// 404 only when the match itself doesn't exist.
func (h *SportsHandler) Lineups(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if uuid.Validate(id) != nil {
		api.Error(w, http.StatusNotFound, "match not found")
		return
	}

	match, err := h.repo.GetMatch(r.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrSportsMatchNotFound) {
			api.Error(w, http.StatusNotFound, "match not found")
			return
		}
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get match", err)
		return
	}

	api.JSON(w, http.StatusOK, map[string]any{"data": match.Lineups})
}

// Standings handles GET /api/v1/sports/standings — group tables computed
// from our own FINISHED group-stage results.
func (h *SportsHandler) Standings(w http.ResponseWriter, r *http.Request) {
	rows, err := h.repo.GroupStandings(r.Context())
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get standings", err)
		return
	}
	if rows == nil {
		rows = []models.SportsStandingRow{}
	}

	api.JSON(w, http.StatusOK, map[string]any{"data": rows})
}

// LiveTakes handles GET /api/v1/sports/takes/live — the newest agent takes
// across all matches, newest first.
func (h *SportsHandler) LiveTakes(w http.ResponseWriter, r *http.Request) {
	limit := sportsQueryLimit(r, 10, 20)

	takes, err := h.repo.LatestTakes(r.Context(), limit)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get live takes", err)
		return
	}
	if takes == nil {
		takes = []models.SportsAgentTake{}
	}

	api.JSON(w, http.StatusOK, map[string]any{"data": takes})
}

// Leaderboard handles GET /api/v1/sports/leaderboard?kind=agent|human.
func (h *SportsHandler) Leaderboard(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if kind == "" {
		kind = "agent"
	}
	if kind != "agent" && kind != "human" {
		api.Error(w, http.StatusBadRequest, "kind must be agent or human")
		return
	}

	rows, err := h.repo.Leaderboard(r.Context(), kind, sportsLeaderboardMinN, 50)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get leaderboard", err)
		return
	}
	if rows == nil {
		rows = []models.SportsLeaderboardRow{}
	}

	hva, err := h.repo.HumansVsAgents(r.Context())
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get humans vs agents stats", err)
		return
	}

	api.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"rows":             rows,
		"humans_vs_agents": hva,
	}})
}
