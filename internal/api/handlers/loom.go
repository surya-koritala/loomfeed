package handlers

import (
	"errors"
	"net/http"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/loom"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// LoomHandler exposes endpoints for the Loom summon system.
//
// Two surfaces today:
//
//   1. GET  /api/v1/loom/summons/{id}  — per-summon polling (legacy /
//      future per-comment summons).
//   2. GET  /api/v1/posts/{id}/loom    — per-post Loom card. Returns
//      the latest post-card summon for a post (the summary that lives
//      above the comments). 404 when none exists yet.
//   3. POST /api/v1/posts/{id}/loom    — explicit "summon Loom for
//      this post" trigger. Used by the post-detail "Summon Loom"
//      button and (internally) by the comment-mention parser.
//
// GETs accept unauthenticated callers: the response is public-context
// (it summarises the public post). POST requires auth so the daily
// per-participant rate limit applies.
type LoomHandler struct {
	repo    *repository.LoomRepo
	manager *loom.Manager
}

func NewLoomHandler(repo *repository.LoomRepo) *LoomHandler {
	return &LoomHandler{repo: repo}
}

// WithManager wires the Loom manager so the POST handler can trigger
// new summons. Nil-safe: without it, POST returns 503 ("Loom is
// disabled in this environment") — consistent with the comment-create
// path's behaviour when ANTHROPIC/LLM creds are absent.
func (h *LoomHandler) WithManager(m *loom.Manager) {
	h.manager = m
}

// Get returns the current state of one summon. Frontend polls this
// every ~800ms while a placeholder is rendered.
func (h *LoomHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "summon id is required")
		return
	}
	s, err := h.repo.GetSummon(r.Context(), id)
	if err != nil || s == nil {
		api.Error(w, http.StatusNotFound, "summon not found")
		return
	}
	api.JSON(w, http.StatusOK, summonPublicView(s))
}

// GetForPost returns the latest post-card summon for a post — the
// thing rendered by the per-post Loom card above the comments.
//
// Returns 200 {state: "none"} when no card exists yet — *not* a 404.
// The browser logs 4xx responses to console as "Failed to load
// resource" and we can't suppress that from JS, so the "no card"
// signal lives in the body, not the status code. Keeps the console
// clean on every post page load.
func (h *LoomHandler) GetForPost(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}
	s, err := h.repo.LatestPostCardForPost(r.Context(), postID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to load loom card")
		return
	}
	if s == nil {
		api.JSON(w, http.StatusOK, map[string]any{"state": "none"})
		return
	}
	api.JSON(w, http.StatusOK, summonPublicView(s))
}

// PostForPost triggers a Loom summon for a post. Auth required (rate-
// limit is per-participant). On a cache hit the worker resolves
// within a tenth of a second; on a miss the LLM call typically lands
// the response in 1-3s. Returns the new summon row's id + state
// immediately so the frontend can switch to polling the card endpoint
// without waiting for inference.
func (h *LoomHandler) PostForPost(w http.ResponseWriter, r *http.Request) {
	if h.manager == nil {
		api.Error(w, http.StatusServiceUnavailable, "loom is not enabled in this environment")
		return
	}
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "authentication required to summon loom")
		return
	}
	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}
	participantID := claims.ParticipantID
	summonID, err := h.manager.Summon(r.Context(), loom.SummonParams{
		ParticipantID: &participantID,
		PostID:        postID,
		// Empty UserMessage falls through Classify() to the default
		// (summarize) — that's the right intent for a "summon for this
		// post" trigger with no further context.
		UserMessage: "summarize",
		PostCard:    true,
	})
	if err != nil {
		switch {
		case errors.Is(err, loom.ErrRateLimited):
			w.Header().Set("Retry-After", "86400")
			api.Error(w, http.StatusTooManyRequests, "daily Loom limit reached — try again tomorrow")
		case errors.Is(err, loom.ErrNoContent):
			api.Error(w, http.StatusBadRequest, "this post has no body to summarize")
		default:
			api.Error(w, http.StatusInternalServerError, "failed to summon loom")
		}
		return
	}
	api.JSON(w, http.StatusAccepted, map[string]any{
		"summon_id": summonID,
		"state":     "pending",
	})
}

// summonPublicView strips operator-side telemetry (prompt, raw error
// codes) and keeps just what the frontend needs to render. Reused by
// both per-summon and per-post-card surfaces so the response shape is
// consistent across endpoints.
func summonPublicView(s *models.LoomSummon) map[string]any {
	return map[string]any{
		"id":               s.ID,
		"state":            s.State,
		"intent":           s.Intent,
		"response":         s.Response,
		"model":            s.Model,
		"reply_comment_id": s.ReplyCommentID,
		"cached":           s.Cached,
		"created_at":       s.CreatedAt,
		"finished_at":      s.FinishedAt,
	}
}
