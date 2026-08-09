package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// FollowHandler handles follow/unfollow endpoints.
type FollowHandler struct {
	follows       *repository.FollowRepo
	notifications *repository.NotificationRepo
	participants  *repository.ParticipantRepo
	blocks        *repository.BlockRepo
}

// NewFollowHandler creates a new FollowHandler.
func NewFollowHandler(follows *repository.FollowRepo) *FollowHandler {
	return &FollowHandler{follows: follows}
}

// WithNotifications wires the notification + participant + block
// repos so a successful follow fires a "new follower" notification
// to the followed account. Optional — when unset, follow is silent
// (the row still gets written; just no inbox ping).
func (h *FollowHandler) WithNotifications(
	notifs *repository.NotificationRepo,
	participants *repository.ParticipantRepo,
	blocks *repository.BlockRepo,
) {
	h.notifications = notifs
	h.participants = participants
	h.blocks = blocks
}

// notifyNewFollower fires a "new_follower" notification on the
// followed account's inbox. Best-effort, non-blocking. Drops if
// the recipient has blocked the follower (Phase 0.2 wired the
// IsBlocked check; reusing it here keeps "blocked = invisible
// across the platform" consistent).
func (h *FollowHandler) notifyNewFollower(followerID, followedID string) {
	if h.notifications == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5)
	defer cancel()
	_ = ctx // (unused, for symmetry with similar paths)

	bg := context.Background()
	if h.blocks != nil {
		if blocked, _ := h.blocks.IsBlocked(bg, followedID, followerID); blocked {
			return
		}
	}
	actorName := "Someone"
	if h.participants != nil {
		if p, err := h.participants.GetByID(bg, followerID); err == nil && p != nil && p.DisplayName != "" {
			actorName = p.DisplayName
		}
	}
	msg := fmt.Sprintf("%s started following you", actorName)
	_ = h.notifications.Create(bg, followedID, "new_follower", &followerID, nil, nil, msg)
}

// Follow handles POST /api/v1/participants/{id}/follow.
func (h *FollowHandler) Follow(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	followedID := r.PathValue("id")
	if followedID == "" {
		api.Error(w, http.StatusBadRequest, "participant id is required")
		return
	}

	if followedID == claims.ParticipantID {
		api.Error(w, http.StatusBadRequest, "you cannot follow yourself")
		return
	}

	if err := h.follows.Follow(r.Context(), claims.ParticipantID, followedID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to follow")
		return
	}

	// Async — never block the response on inbox writes.
	go h.notifyNewFollower(claims.ParticipantID, followedID)

	api.JSON(w, http.StatusOK, map[string]string{"status": "followed"})
}

// Unfollow handles DELETE /api/v1/participants/{id}/follow.
func (h *FollowHandler) Unfollow(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	followedID := r.PathValue("id")
	if followedID == "" {
		api.Error(w, http.StatusBadRequest, "participant id is required")
		return
	}

	if err := h.follows.Unfollow(r.Context(), claims.ParticipantID, followedID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to unfollow")
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "unfollowed"})
}

// IsFollowing handles GET /api/v1/participants/{id}/follow — checks if the
// authenticated user is following the given participant.
func (h *FollowHandler) IsFollowing(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	followedID := r.PathValue("id")
	if followedID == "" {
		api.Error(w, http.StatusBadRequest, "participant id is required")
		return
	}

	following, err := h.follows.IsFollowing(r.Context(), claims.ParticipantID, followedID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to check follow status")
		return
	}

	api.JSON(w, http.StatusOK, map[string]bool{"following": following})
}

// ListFollowing handles GET /api/v1/participants/{id}/following — returns
// participants that the given participant is following.
func (h *FollowHandler) ListFollowing(w http.ResponseWriter, r *http.Request) {
	participantID := r.PathValue("id")
	if participantID == "" {
		api.Error(w, http.StatusBadRequest, "participant id is required")
		return
	}

	limit := parseIntQuery(r, "limit", 25)
	offset := parseIntQuery(r, "offset", 0)

	follows, total, err := h.follows.ListFollowing(r.Context(), participantID, limit, offset)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list following")
		return
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"data":  follows,
		"total": total,
	})
}

// ListFollowers handles GET /api/v1/participants/{id}/followers — returns
// participants that follow the given participant.
func (h *FollowHandler) ListFollowers(w http.ResponseWriter, r *http.Request) {
	participantID := r.PathValue("id")
	if participantID == "" {
		api.Error(w, http.StatusBadRequest, "participant id is required")
		return
	}

	limit := parseIntQuery(r, "limit", 25)
	offset := parseIntQuery(r, "offset", 0)

	followers, total, err := h.follows.ListFollowers(r.Context(), participantID, limit, offset)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list followers")
		return
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"data":  followers,
		"total": total,
	})
}
