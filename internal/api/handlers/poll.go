package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/config"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// PollHandler handles poll endpoints.
type PollHandler struct {
	polls *repository.PollRepo
	cfg   *config.Config
}

// NewPollHandler creates a new PollHandler.
func NewPollHandler(polls *repository.PollRepo, cfg *config.Config) *PollHandler {
	return &PollHandler{polls: polls, cfg: cfg}
}

// createPollRequest is the request body for creating a poll.
type createPollRequest struct {
	Options  []string `json:"options"`
	Deadline *string  `json:"deadline,omitempty"`
}

// votePollRequest is the request body for casting a poll vote.
type votePollRequest struct {
	OptionID string `json:"option_id"`
}

// Create handles POST /api/v1/posts/{id}/poll.
func (h *PollHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req createPollRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Options) < 2 {
		api.Error(w, http.StatusBadRequest, "at least 2 options are required")
		return
	}
	if len(req.Options) > 10 {
		api.Error(w, http.StatusBadRequest, "at most 10 options are allowed")
		return
	}

	// Validate that options are non-empty
	for i, opt := range req.Options {
		req.Options[i] = strings.TrimSpace(opt)
		if req.Options[i] == "" {
			api.Error(w, http.StatusBadRequest, "option text cannot be empty")
			return
		}
	}

	var deadline *time.Time
	if req.Deadline != nil && *req.Deadline != "" {
		t, err := time.Parse(time.RFC3339, *req.Deadline)
		if err != nil {
			api.Error(w, http.StatusBadRequest, "invalid deadline format, use RFC3339")
			return
		}
		deadline = &t
	}

	// Check if a poll already exists for this post
	existing, err := h.polls.GetByPostID(r.Context(), postID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to check existing poll")
		return
	}
	if existing != nil {
		api.Error(w, http.StatusConflict, "poll already exists for this post")
		return
	}

	poll, err := h.polls.CreatePoll(r.Context(), postID, req.Options, deadline)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to create poll")
		return
	}

	api.JSON(w, http.StatusCreated, poll)
}

// Vote handles POST /api/v1/posts/{id}/poll/vote.
func (h *PollHandler) Vote(w http.ResponseWriter, r *http.Request) {
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

	var req votePollRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.OptionID == "" {
		api.Error(w, http.StatusBadRequest, "option_id is required")
		return
	}

	// Get the poll for this post
	poll, err := h.polls.GetByPostID(r.Context(), postID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to get poll")
		return
	}
	if poll == nil {
		api.Error(w, http.StatusNotFound, "no poll found for this post")
		return
	}

	// Check deadline
	if poll.Deadline != nil && time.Now().After(*poll.Deadline) {
		api.Error(w, http.StatusBadRequest, "poll has ended")
		return
	}

	// Verify the option belongs to this poll
	validOption := false
	for _, opt := range poll.Options {
		if opt.ID == req.OptionID {
			validOption = true
			break
		}
	}
	if !validOption {
		api.Error(w, http.StatusBadRequest, "invalid option_id for this poll")
		return
	}

	// Check if user already voted
	existingVote, err := h.polls.GetUserVote(r.Context(), poll.ID, claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to check existing vote")
		return
	}
	if existingVote != nil {
		api.Error(w, http.StatusConflict, "you have already voted on this poll")
		return
	}

	if err := h.polls.Vote(r.Context(), poll.ID, req.OptionID, claims.ParticipantID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to cast vote")
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "voted"})
}

// Get handles GET /api/v1/posts/{id}/poll.
func (h *PollHandler) Get(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}

	poll, err := h.polls.GetByPostID(r.Context(), postID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to get poll")
		return
	}
	// 200 + null payload for "no poll attached" so the frontend can
	// render PollCard on every post detail and just ignore the null,
	// without flooding the browser console with 404s for the common
	// case (most posts don't have polls).
	if poll == nil {
		api.JSON(w, http.StatusOK, nil)
		return
	}

	// If authenticated, include the user's vote
	claims := middleware.GetClaims(r.Context())
	if claims != nil {
		userVote, err := h.polls.GetUserVote(r.Context(), poll.ID, claims.ParticipantID)
		if err == nil {
			poll.UserVote = userVote
		}
	}

	api.JSON(w, http.StatusOK, poll)
}
