package handlers

import (
	"net/http"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/events"
	"github.com/RoamXAI/loomfeed/internal/repository"
	"github.com/RoamXAI/loomfeed/internal/scorecard"
)

// EpistemicHandler handles epistemic status vote endpoints.
type EpistemicHandler struct {
	epistemic *repository.EpistemicRepo
	posts     *repository.PostRepo
	hub       *events.Hub
}

// NewEpistemicHandler creates a new EpistemicHandler.
func NewEpistemicHandler(epistemic *repository.EpistemicRepo) *EpistemicHandler {
	return &EpistemicHandler{epistemic: epistemic}
}

// WithScorecardTrigger sets the post repo and event hub for scorecard triggers.
func (h *EpistemicHandler) WithScorecardTrigger(posts *repository.PostRepo, hub *events.Hub) {
	h.posts = posts
	h.hub = hub
}

// epistemicVoteRequest is the request body for voting on epistemic status.
type epistemicVoteRequest struct {
	Status string `json:"status"`
}

// Vote handles POST /api/v1/posts/{id}/epistemic.
func (h *EpistemicHandler) Vote(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}

	var req epistemicVoteRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !repository.IsValidStatus(req.Status) {
		api.Error(w, http.StatusBadRequest, "status must be one of: hypothesis, supported, contested, refuted, consensus")
		return
	}

	if err := h.epistemic.Vote(r.Context(), postID, claims.ParticipantID, req.Status); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to cast epistemic vote")
		return
	}

	// Trigger scorecard recomputation for the post author
	if h.hub != nil && h.posts != nil {
		if post, err := h.posts.GetByID(r.Context(), postID); err == nil && post.AuthorID != "" {
			scorecard.TriggerCompute(h.hub, post.AuthorID)
		}
	}

	// Return the updated status
	result, err := h.epistemic.GetPostStatus(r.Context(), postID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to get epistemic status")
		return
	}

	result.UserVote = req.Status
	api.JSON(w, http.StatusOK, result)
}

// Get handles GET /api/v1/posts/{id}/epistemic.
func (h *EpistemicHandler) Get(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}

	result, err := h.epistemic.GetPostStatus(r.Context(), postID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to get epistemic status")
		return
	}
	if result == nil {
		api.Error(w, http.StatusNotFound, "post not found")
		return
	}

	// If authenticated, include the user's vote
	claims := middleware.GetClaims(r.Context())
	if claims != nil {
		userVote, err := h.epistemic.GetUserVote(r.Context(), postID, claims.ParticipantID)
		if err == nil && userVote != "" {
			result.UserVote = userVote
		}
	}

	api.JSON(w, http.StatusOK, result)
}
