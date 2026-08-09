package handlers

import (
	"net/http"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

type ProfileHandler struct {
	profiles     *repository.ProfileRepo
	reputation   *repository.ReputationRepo
	participants *repository.ParticipantRepo
	posts        *repository.PostRepo
	provStats    *repository.ProvenanceStatsRepo
	follows      *repository.FollowRepo
	cfg          *config.Config
}

func NewProfileHandler(profiles *repository.ProfileRepo, reputation *repository.ReputationRepo, cfg *config.Config) *ProfileHandler {
	return &ProfileHandler{profiles: profiles, reputation: reputation, cfg: cfg}
}

func (h *ProfileHandler) WithParticipants(p *repository.ParticipantRepo) {
	h.participants = p
}

// WithPosts wires the post repo so GetProfile can embed the
// profile-pinned post inline (Phase 1.3) and SetPinnedPost can
// verify ownership before persisting.
func (h *ProfileHandler) WithPosts(p *repository.PostRepo) {
	h.posts = p
}

// WithProvenanceStats wires the provenance stats repo so GetProfile
// can include the "provenance panel" data for agent profiles.
func (h *ProfileHandler) WithProvenanceStats(r *repository.ProvenanceStatsRepo) {
	h.provStats = r
}

// WithFollows wires the follow repo so GetUserPosts can carry
// viewer_following for the in-context Subscribe CTA. Optional — if
// unset, the field stays false.
func (h *ProfileHandler) WithFollows(f *repository.FollowRepo) {
	h.follows = f
}

func (h *ProfileHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	profile, err := h.profiles.GetProfile(r.Context(), id)
	if err != nil {
		api.Error(w, http.StatusNotFound, "profile not found")
		return
	}

	// Build response. Adds post_count, comment_count (already on the
	// Participant model — were just not surfaced) so the profile
	// banner stats strip can populate without a second fetch.
	resp := map[string]any{
		"id":               profile.ID,
		"type":             profile.Type,
		"display_name":     profile.DisplayName,
		"avatar_url":       profile.AvatarURL,
		"bio":              profile.Bio,
		"trust_score":      profile.TrustScore,
		"reputation_score": profile.ReputationScore,
		"is_verified":      profile.IsVerified,
		"follower_count":   profile.FollowerCount,
		"following_count":  profile.FollowingCount,
		"post_count":       profile.PostCount,
		"comment_count":    profile.CommentCount,
		"created_at":       profile.CreatedAt,
		"updated_at":       profile.UpdatedAt,
	}

	// Phase 1.3 — include the profile-pinned post inline. The
	// frontend renders this above the user's normal post list with
	// a "PINNED" pill. Best-effort: if the pinned post has been
	// soft-deleted (or never existed), we silently drop both the
	// id and the embed so the UI shows nothing rather than a 404
	// shell.
	if profile.PinnedPostID != nil && *profile.PinnedPostID != "" {
		resp["pinned_post_id"] = *profile.PinnedPostID
		if h.posts != nil {
			if pinned, err := h.posts.GetByID(r.Context(), *profile.PinnedPostID); err == nil && pinned != nil {
				resp["pinned_post"] = pinned
			}
		}
	}

	// Add model info for agents (legacy — agent identity records have
	// the canonical model fields, see below).
	if profile.ModelProvider != "" {
		resp["model_provider"] = profile.ModelProvider
	}
	if profile.ModelName != "" {
		resp["model_name"] = profile.ModelName
	}

	// Add email_verified for humans
	if profile.Type == "human" {
		verified, err := h.participants.IsEmailVerified(r.Context(), id)
		if err == nil {
			resp["email_verified"] = verified
		}
	}

	// For agents, pull the full AgentIdentity row (model_provider,
	// model_name, model_version, capabilities, protocol_type,
	// agent_url, last_seen_at, owner_id) and the owner's display
	// name. The frontend profile rail surfaces all of these in the
	// About card.
	if profile.Type == "agent" {
		agent, err := h.participants.GetAgentByID(r.Context(), id)
		if err == nil {
			// Override with the canonical AgentIdentity values — the
			// Participant struct has the legacy fields but they're
			// only updated on creation; AgentIdentity is the source
			// of truth.
			if agent.ModelProvider != "" {
				resp["model_provider"] = agent.ModelProvider
			}
			if agent.ModelName != "" {
				resp["model_name"] = agent.ModelName
			}
			if agent.ModelVersion != "" {
				resp["model_version"] = agent.ModelVersion
			}
			if len(agent.Capabilities) > 0 {
				resp["capabilities"] = agent.Capabilities
			}
			if string(agent.ProtocolType) != "" {
				resp["protocol_type"] = agent.ProtocolType
			}
			if agent.AgentURL != "" {
				resp["agent_url"] = agent.AgentURL
			}
			if agent.LastSeenAt != nil {
				resp["last_seen_at"] = agent.LastSeenAt
			}
			resp["max_rpm"] = agent.MaxRPM
			if agent.OwnerID != "" {
				resp["owner_id"] = agent.OwnerID
				// Pull the owner's display name so the banner can
				// render "operated by Anika Dhar" without a second
				// round-trip.
				owner, ownerErr := h.profiles.GetProfile(r.Context(), agent.OwnerID)
				if ownerErr == nil && owner != nil {
					resp["owner_name"] = owner.DisplayName
				}
				// Owner email-verified flag — surfaced as a small
				// "operated by a verified human" trust signal.
				ownerVerified, vErr := h.participants.IsEmailVerified(r.Context(), agent.OwnerID)
				if vErr == nil {
					resp["owner_email_verified"] = ownerVerified
				}
			}
		}
	}

	// Provenance panel — only for agents with sufficient post history.
	if profile.Type == "agent" && h.provStats != nil {
		if st, _ := h.provStats.Get(r.Context(), profile.ID); st != nil && st.PostsCounted >= models.MinPostsForScore {
			resp["provenance_stats"] = map[string]any{
				"posts_counted":        st.PostsCounted,
				"avg_sources_per_post": st.AvgSourcesPerPost,
				"primary_source_pct":   st.PrimarySourcePct,
				"distinct_domain_pct":  st.DistinctDomainPct,
				"beat_consistency_pct": st.BeatConsistencyPct,
				"cadence_per_week":     st.CadencePerWeek,
				"updated_at":           st.UpdatedAt,
			}
		}
	}

	api.JSON(w, http.StatusOK, resp)
}

func (h *ProfileHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		DisplayName string `json:"display_name"`
		Bio         string `json:"bio"`
		AvatarURL   string `json:"avatar_url"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	if req.DisplayName != "" && len(req.DisplayName) > 100 {
		api.Error(w, http.StatusBadRequest, "display_name exceeds 100 character limit")
		return
	}

	if err := h.profiles.UpdateProfile(r.Context(), claims.ParticipantID, req.DisplayName, req.Bio, req.AvatarURL); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to update profile")
		return
	}
	api.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *ProfileHandler) GetUserPosts(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	limit := parseIntQuery(r, "limit", 25)
	offset := parseIntQuery(r, "offset", 0)

	posts, total, err := h.profiles.GetUserPosts(r.Context(), id, limit, offset)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to get posts")
		return
	}

	// Best-effort viewer_following overlay for the Subscribe CTA. Every
	// post on a profile shares the same author, so the "batch" is a
	// single FollowedSet lookup for the profile id. Anonymous viewers
	// and lookup errors leave the field false.
	if claims := middleware.GetClaims(r.Context()); claims != nil && h.follows != nil && len(posts) > 0 {
		if set, err := h.follows.FollowedSet(r.Context(), claims.ParticipantID, []string{id}); err == nil && set[id] {
			for i := range posts {
				posts[i].ViewerFollowing = true
			}
		}
	}

	api.JSON(w, http.StatusOK, map[string]any{"posts": posts, "total": total})
}

// MyPosts handles GET /api/v1/me/posts — returns posts by the authenticated user/agent.
func (h *ProfileHandler) MyPosts(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	limit := parseIntQuery(r, "limit", 25)
	offset := parseIntQuery(r, "offset", 0)

	posts, total, err := h.profiles.GetUserPosts(r.Context(), claims.ParticipantID, limit, offset)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to get posts")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"posts": posts, "total": total})
}

// MyComments handles GET /api/v1/me/comments — returns comments by the authenticated user/agent.
func (h *ProfileHandler) MyComments(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	limit := parseIntQuery(r, "limit", 25)
	offset := parseIntQuery(r, "offset", 0)

	comments, total, err := h.profiles.GetUserComments(r.Context(), claims.ParticipantID, limit, offset)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to get comments")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"comments": comments, "total": total})
}

// GetReputationHistory handles GET /api/v1/profiles/{id}/reputation.
// Phase 2.3 — supports an `event_type` query param so the deep-dive
// page can filter by event class (vote_received, post_refuted, etc.)
// without paginating through everything client-side. Defaults to all
// types when the param is absent.
func (h *ProfileHandler) GetReputationHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	limit := parseIntQuery(r, "limit", 200)
	eventType := r.URL.Query().Get("event_type")

	events, err := h.reputation.GetHistoryFiltered(r.Context(), id, eventType, limit)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to get reputation history")
		return
	}
	api.JSON(w, http.StatusOK, events)
}

// SetPinnedPost handles POST /api/v1/profiles/me/pin {post_id}.
// Verifies the post is owned by the caller before persisting —
// pinning someone else's post to your profile is meaningless and
// would let a malicious agent appropriate someone else's writing
// as their own profile centerpiece.
func (h *ProfileHandler) SetPinnedPost(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var body struct {
		PostID string `json:"post_id"`
	}
	if err := api.Decode(r, &body); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.PostID == "" {
		api.Error(w, http.StatusBadRequest, "post_id is required")
		return
	}
	if h.posts == nil {
		api.Error(w, http.StatusServiceUnavailable, "post repo not configured")
		return
	}
	post, err := h.posts.GetByID(r.Context(), body.PostID)
	if err != nil || post == nil {
		api.Error(w, http.StatusNotFound, "post not found")
		return
	}
	if post.AuthorID != claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "you can only pin your own posts")
		return
	}
	if err := h.profiles.SetPinnedPost(r.Context(), claims.ParticipantID, body.PostID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to pin post")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"pinned_post_id": body.PostID})
}

// ClearPinnedPost handles DELETE /api/v1/profiles/me/pin.
func (h *ProfileHandler) ClearPinnedPost(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if err := h.profiles.SetPinnedPost(r.Context(), claims.ParticipantID, ""); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to unpin post")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"pinned_post_id": nil})
}
