package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// ChallengeHandler handles challenge endpoints.
type ChallengeHandler struct {
	challenges *repository.ChallengeRepo
	reputation *repository.ReputationRepo
}

// NewChallengeHandler creates a new ChallengeHandler.
func NewChallengeHandler(challenges *repository.ChallengeRepo, reputation *repository.ReputationRepo) *ChallengeHandler {
	return &ChallengeHandler{
		challenges: challenges,
		reputation: reputation,
	}
}

// Create handles POST /api/v1/challenges.
func (h *ChallengeHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		Title        string   `json:"title"`
		Body         string   `json:"body"`
		CommunityID  string   `json:"community_id"`
		Deadline     *string  `json:"deadline"`
		Capabilities []string `json:"capabilities"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Title == "" || req.Body == "" || req.CommunityID == "" {
		api.Error(w, http.StatusBadRequest, "title, body, and community_id are required")
		return
	}

	var deadline *time.Time
	if req.Deadline != nil && *req.Deadline != "" {
		t, err := time.Parse(time.RFC3339, *req.Deadline)
		if err != nil {
			api.Error(w, http.StatusBadRequest, "deadline must be in RFC3339 format")
			return
		}
		deadline = &t
	}

	challenge, err := h.challenges.Create(r.Context(), req.Title, req.Body, req.CommunityID,
		claims.ParticipantID, deadline, req.Capabilities)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to create challenge")
		return
	}

	api.JSON(w, http.StatusCreated, challenge)
}

// List handles GET /api/v1/challenges.
func (h *ChallengeHandler) List(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit := 25
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

	challenges, err := h.challenges.List(r.Context(), status, limit, offset)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list challenges")
		return
	}

	if challenges == nil {
		challenges = []repository.Challenge{}
	}
	api.JSON(w, http.StatusOK, challenges)
}

// Get handles GET /api/v1/challenges/{id}.
func (h *ChallengeHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	challenge, err := h.challenges.GetByID(r.Context(), id)
	if err != nil {
		if err.Error() == "get challenge: "+pgx.ErrNoRows.Error() {
			api.Error(w, http.StatusNotFound, "challenge not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to get challenge")
		return
	}

	submissions, err := h.challenges.ListSubmissions(r.Context(), id)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to get submissions")
		return
	}

	if submissions == nil {
		submissions = []repository.ChallengeSubmission{}
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"challenge":   challenge,
		"submissions": submissions,
	})
}

// Submit handles POST /api/v1/challenges/{id}/submit.
func (h *ChallengeHandler) Submit(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	var req struct {
		Body string `json:"body"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Body == "" {
		api.Error(w, http.StatusBadRequest, "body is required")
		return
	}

	submission, err := h.challenges.Submit(r.Context(), id, claims.ParticipantID, req.Body)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to submit — you may have already submitted")
		return
	}

	api.JSON(w, http.StatusCreated, submission)
}

// VoteSubmission handles POST /api/v1/challenges/{id}/submissions/{subId}/vote.
func (h *ChallengeHandler) VoteSubmission(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	subID := r.PathValue("subId")
	if subID == "" {
		api.Error(w, http.StatusBadRequest, "submission id is required")
		return
	}

	newScore, err := h.challenges.VoteSubmission(r.Context(), subID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to vote on submission")
		return
	}

	api.JSON(w, http.StatusOK, map[string]int{"score": newScore})
}

// PickWinner handles POST /api/v1/challenges/{id}/winner.
func (h *ChallengeHandler) PickWinner(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	var req struct {
		SubmissionID string `json:"submission_id"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SubmissionID == "" {
		api.Error(w, http.StatusBadRequest, "submission_id is required")
		return
	}

	// Verify the caller is the creator of the challenge
	challenge, err := h.challenges.GetByID(r.Context(), id)
	if err != nil {
		api.Error(w, http.StatusNotFound, "challenge not found")
		return
	}

	if challenge.CreatedBy != claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "only the challenge creator can pick a winner")
		return
	}

	if challenge.Status == "closed" {
		api.Error(w, http.StatusBadRequest, "challenge is already closed")
		return
	}

	winnerParticipantID, err := h.challenges.PickWinner(r.Context(), id, req.SubmissionID, claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to pick winner")
		return
	}

	// Award +5 reputation to the winner
	go func() {
		ctx := context.Background()
		_ = h.reputation.RecordEvent(ctx, winnerParticipantID, repository.EventAcceptedAnswer, 5.0)
	}()

	api.JSON(w, http.StatusOK, map[string]string{
		"status":     "ok",
		"winner_id":  winnerParticipantID,
		"message":    "winner selected and challenge closed",
	})
}
