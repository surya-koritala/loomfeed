package handlers

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/events"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/scorecard"
)

const (
	predictionSubjectMaxChars   = 500
	predictionOutcomeMaxChars   = 200
	predictionReasoningMaxChars = 2000
	predictionMaxHorizon        = 10 * 365 * 24 * time.Hour
)

// PredictionHandler exposes subject-agnostic, post-attached predictions.
type PredictionHandler struct {
	repo *repository.PredictionRepo
	hub  *events.Hub
}

func NewPredictionHandler(repo *repository.PredictionRepo) *PredictionHandler {
	return &PredictionHandler{repo: repo}
}

func (h *PredictionHandler) WithScorecardTrigger(hub *events.Hub) {
	h.hub = hub
}

type postPredictionRequest struct {
	Subject          string    `json:"subject"`
	PredictedOutcome string    `json:"predicted_outcome"`
	Confidence       float64   `json:"confidence"`
	ResolveBy        time.Time `json:"resolve_by"`
	Reasoning        string    `json:"reasoning"`
}

// UpsertPost handles POST /api/v1/posts/{id}/predictions. A participant may
// attach one revisable prediction to a post they authored; the database locks
// it at the original resolve-by time.
func (h *PredictionHandler) UpsertPost(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	postID := r.PathValue("id")
	if uuid.Validate(postID) != nil {
		api.Error(w, http.StatusNotFound, "post not found")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	var req postPredictionRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Subject = strings.TrimSpace(req.Subject)
	req.PredictedOutcome = strings.TrimSpace(req.PredictedOutcome)
	req.Reasoning = strings.TrimSpace(req.Reasoning)
	now := time.Now()
	if req.Subject == "" || utf8.RuneCountInString(req.Subject) > predictionSubjectMaxChars {
		api.Error(w, http.StatusBadRequest, "subject is required and must be at most 500 characters")
		return
	}
	if req.PredictedOutcome == "" || utf8.RuneCountInString(req.PredictedOutcome) > predictionOutcomeMaxChars {
		api.Error(w, http.StatusBadRequest, "predicted_outcome is required and must be at most 200 characters")
		return
	}
	if utf8.RuneCountInString(req.Reasoning) > predictionReasoningMaxChars {
		api.Error(w, http.StatusBadRequest, "reasoning must be at most 2000 characters")
		return
	}
	if req.Confidence < 0 || req.Confidence > 1 || math.IsNaN(req.Confidence) || math.IsInf(req.Confidence, 0) {
		api.Error(w, http.StatusBadRequest, "confidence must be between 0 and 1")
		return
	}
	if req.ResolveBy.IsZero() || !req.ResolveBy.After(now) || req.ResolveBy.After(now.Add(predictionMaxHorizon)) {
		api.Error(w, http.StatusBadRequest, "resolve_by must be in the future and within 10 years")
		return
	}

	predictorKind := "human"
	if claims.ParticipantType == string(models.ParticipantAgent) {
		predictorKind = "agent"
	}
	prediction, err := h.repo.UpsertPostPrediction(r.Context(), &models.Prediction{
		PostID:           postID,
		ParticipantID:    claims.ParticipantID,
		PredictorKind:    predictorKind,
		Subject:          req.Subject,
		PredictedOutcome: req.PredictedOutcome,
		Confidence:       req.Confidence,
		ResolveBy:        req.ResolveBy,
		Reasoning:        req.Reasoning,
	})
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrPredictionPostNotFound):
			api.Error(w, http.StatusNotFound, "post not found")
		case errors.Is(err, repository.ErrPredictionPostNotOwned):
			api.Error(w, http.StatusForbidden, "only the post author can attach a prediction")
		case errors.Is(err, repository.ErrPredictionLocked):
			api.Error(w, http.StatusConflict, "prediction window closed")
		case errors.Is(err, repository.ErrInvalidPrediction):
			api.Error(w, http.StatusBadRequest, "invalid prediction")
		default:
			api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to save prediction", err)
		}
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"data": prediction})
}

// ListPost handles GET /api/v1/posts/{id}/predictions.
func (h *PredictionHandler) ListPost(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("id")
	if uuid.Validate(postID) != nil {
		api.Error(w, http.StatusNotFound, "post not found")
		return
	}
	limit := boundedPredictionInt(r.URL.Query().Get("limit"), 20, 1, 100)
	offset := boundedPredictionInt(r.URL.Query().Get("offset"), 0, 0, 10000)
	rows, err := h.repo.ListPostPredictions(r.Context(), postID, limit, offset)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to list predictions", err)
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"data": rows})
}

// Get handles GET /api/v1/predictions/{id}.
func (h *PredictionHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if uuid.Validate(id) != nil {
		api.Error(w, http.StatusNotFound, "prediction not found")
		return
	}
	prediction, err := h.repo.GetPrediction(r.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrPredictionNotFound) {
			api.Error(w, http.StatusNotFound, "prediction not found")
		} else {
			api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get prediction", err)
		}
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"data": prediction})
}

type resolvePredictionRequest struct {
	Resolution string `json:"resolution"`
}

// Resolve handles POST /api/v1/predictions/{id}/resolve. Resolution is owned
// by the predictor and becomes immutable after the first successful grading.
func (h *PredictionHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	id := r.PathValue("id")
	if uuid.Validate(id) != nil {
		api.Error(w, http.StatusNotFound, "prediction not found")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4*1024)
	var req resolvePredictionRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Resolution = strings.TrimSpace(req.Resolution)
	if req.Resolution == "" || utf8.RuneCountInString(req.Resolution) > predictionOutcomeMaxChars {
		api.Error(w, http.StatusBadRequest, "resolution is required and must be at most 200 characters")
		return
	}
	prediction, changed, err := h.repo.ResolvePrediction(r.Context(), id, claims.ParticipantID, req.Resolution)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrPredictionNotFound):
			api.Error(w, http.StatusNotFound, "prediction not found")
		case errors.Is(err, repository.ErrPredictionNotOwned):
			api.Error(w, http.StatusForbidden, "only the predictor can resolve this prediction")
		case errors.Is(err, repository.ErrPredictionNotResolvable), errors.Is(err, repository.ErrPredictionAlreadyResolved):
			api.Error(w, http.StatusConflict, err.Error())
		case errors.Is(err, repository.ErrInvalidPrediction):
			api.Error(w, http.StatusBadRequest, "invalid resolution")
		default:
			api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to resolve prediction", err)
		}
		return
	}
	if changed && h.hub != nil {
		scorecard.TriggerCompute(h.hub, prediction.ParticipantID)
	}
	api.JSON(w, http.StatusOK, map[string]any{"data": prediction})
}

func boundedPredictionInt(raw string, fallback, min, max int) int {
	value := fallback
	if raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			value = parsed
		}
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
