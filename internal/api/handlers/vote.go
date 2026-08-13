package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/cache"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/events"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/ratelimit"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/scorecard"
	"github.com/surya-koritala/loomfeed/internal/webhook"
)

// VoteHandler handles vote endpoints.
type VoteHandler struct {
	votes       *repository.VoteRepo
	posts       *repository.PostRepo
	comments    *repository.CommentRepo
	reputation  *repository.ReputationRepo
	rateLimiter ratelimit.Limiter
	cfg         *config.Config
	dispatcher  webhookEventDispatcher
	hub         *events.Hub
	cache       *cache.RedisCache
}

// WithCache sets the Redis cache for cache invalidation on votes.
func (h *VoteHandler) WithCache(c *cache.RedisCache) {
	h.cache = c
}

// NewVoteHandler creates a new VoteHandler.
func NewVoteHandler(votes *repository.VoteRepo, posts *repository.PostRepo, comments *repository.CommentRepo, reputation *repository.ReputationRepo, cfg *config.Config) *VoteHandler {
	return &VoteHandler{
		votes:      votes,
		posts:      posts,
		comments:   comments,
		reputation: reputation,
		cfg:        cfg,
	}
}

// WithRateLimiter sets the rate limiter for vote casting.
func (h *VoteHandler) WithRateLimiter(rl ratelimit.Limiter) {
	h.rateLimiter = rl
}

// WithWebhook sets the webhook dispatcher and event hub.
func (h *VoteHandler) WithWebhook(dispatcher webhookEventDispatcher, hub *events.Hub) {
	h.dispatcher = dispatcher
	h.hub = hub
}

// Cast handles POST /api/v1/votes.
func (h *VoteHandler) Cast(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Rate limiting per participant
	if h.rateLimiter != nil {
		if !h.rateLimiter.Allow(claims.ParticipantID) {
			remaining := h.rateLimiter.Remaining(claims.ParticipantID)
			w.Header().Set("Retry-After", "60")
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			api.Error(w, http.StatusTooManyRequests, "rate limit exceeded: max 30 votes per minute")
			return
		}
	}

	var req models.VoteRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TargetID == "" {
		api.Error(w, http.StatusBadRequest, "target_id is required")
		return
	}

	if req.TargetType != "post" && req.TargetType != "comment" {
		api.Error(w, http.StatusBadRequest, "target_type must be 'post' or 'comment'")
		return
	}

	if req.Direction != "up" && req.Direction != "down" {
		api.Error(w, http.StatusBadRequest, "direction must be 'up' or 'down'")
		return
	}

	vote := &models.Vote{
		TargetID:   req.TargetID,
		TargetType: models.TargetType(req.TargetType),
		VoterID:    claims.ParticipantID,
		VoterType:  models.ParticipantType(claims.ParticipantType),
		Direction:  models.VoteDirection(req.Direction),
	}

	// Look up the content author so we can do vote + reputation in a single transaction.
	var authorID string
	var delta float64

	if req.TargetType == "post" {
		post, err := h.posts.GetByID(r.Context(), req.TargetID)
		if err == nil {
			authorID = post.AuthorID
		}
		if req.Direction == "up" {
			delta = 0.5
		} else {
			delta = -0.3
		}
	} else {
		comment, err := h.comments.GetByID(r.Context(), req.TargetID)
		if err == nil {
			authorID = comment.AuthorID
		}
		if req.Direction == "up" {
			delta = 0.3
		} else {
			delta = -0.2
		}
	}

	eventType := repository.EventUpvoteReceived
	if req.Direction == "down" {
		eventType = repository.EventDownvoteReceived
	}

	// Merged transaction: vote + reputation in a single BEGIN/COMMIT
	newScore, voteActive, webhookVisible, err := h.votes.CastWithReputation(r.Context(), vote, authorID, eventType, delta)
	if err != nil {
		slog.Error("vote cast failed", "error", err, "target_id", req.TargetID, "voter_id", claims.ParticipantID)
		api.Error(w, http.StatusInternalServerError, "failed to cast vote")
		return
	}

	// Invalidate feed caches — O(1) version bump, old keys age out
	// via their existing TTL.
	if h.cache != nil {
		_ = h.cache.BumpVersion(r.Context(), "feed")
	}

	// Trigger scorecard recomputation for the content author
	if h.hub != nil && authorID != "" {
		scorecard.TriggerCompute(h.hub, authorID)
	}

	// Dispatch webhook + SSE for vote.received (non-blocking)
	if h.dispatcher != nil && webhookVisible && voteActive && authorID != "" && authorID != claims.ParticipantID && req.Direction == "up" {
		payload := map[string]any{
			"target_id":   req.TargetID,
			"target_type": req.TargetType,
			"voter_id":    claims.ParticipantID,
			"direction":   req.Direction,
		}
		dispatchWebhookFallback(h.dispatcher, webhook.EventVoteReceived, payload)
		if h.hub != nil {
			go func() {
				data, _ := json.Marshal(payload)
				h.hub.Publish(authorID, events.Event{Type: "vote.received", Data: string(data)})
			}()
		}
	}

	api.JSON(w, http.StatusOK, map[string]int{"vote_score": newScore})
}
