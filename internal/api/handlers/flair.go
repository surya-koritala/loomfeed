package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

type FlairHandler struct {
	flairs      *repository.FlairRepo
	communities *repository.CommunityRepo
	moderation  *repository.ModerationRepo
}

func NewFlairHandler(flairs *repository.FlairRepo, communities *repository.CommunityRepo, moderation *repository.ModerationRepo) *FlairHandler {
	return &FlairHandler{flairs: flairs, communities: communities, moderation: moderation}
}

// isMod returns true if the user is the community creator or a moderator/admin.
func (h *FlairHandler) isMod(r *http.Request, communityID, userID, creatorID string) bool {
	if creatorID == userID {
		return true
	}
	ok, _ := h.moderation.IsModerator(r.Context(), communityID, userID)
	return ok
}

// List handles GET /api/v1/communities/{slug}/flairs
func (h *FlairHandler) List(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		api.Error(w, http.StatusBadRequest, "slug is required")
		return
	}
	c, err := h.communities.GetBySlug(r.Context(), slug)
	if err != nil {
		api.Error(w, http.StatusNotFound, "community not found")
		return
	}
	flairs, err := h.flairs.ListCommunityFlairs(r.Context(), c.ID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list flairs")
		return
	}
	if flairs == nil {
		flairs = []repository.CommunityFlair{}
	}
	api.JSON(w, http.StatusOK, map[string]any{"flairs": flairs})
}

// Create handles POST /api/v1/communities/{slug}/flairs (mod-only)
func (h *FlairHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	slug := r.PathValue("slug")
	c, err := h.communities.GetBySlug(r.Context(), slug)
	if err != nil {
		api.Error(w, http.StatusNotFound, "community not found")
		return
	}

	// Only creator/admin/moderator can create flairs
	if !h.isMod(r, c.ID, claims.ParticipantID, c.CreatedBy) {
		api.Error(w, http.StatusForbidden, "moderators only")
		return
	}

	var req struct {
		Label   string `json:"label"`
		Color   string `json:"color"`
		ModOnly bool   `json:"mod_only"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Label == "" {
		api.Error(w, http.StatusBadRequest, "label is required")
		return
	}
	if len(req.Label) > 40 {
		api.Error(w, http.StatusBadRequest, "label must be 40 characters or fewer")
		return
	}
	validColors := map[string]bool{"gray": true, "coral": true, "indigo": true, "teal": true, "amber": true, "rose": true}
	if req.Color != "" && !validColors[req.Color] {
		api.Error(w, http.StatusBadRequest, "invalid color")
		return
	}
	f, err := h.flairs.CreateFlair(r.Context(), c.ID, req.Label, req.Color, req.ModOnly)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to create flair")
		return
	}
	api.JSON(w, http.StatusCreated, f)
}

// Assign handles POST /api/v1/communities/{slug}/flair — self-assignment or mod-assignment.
// Body: { "flair_id": "...", "participant_id": "..." (optional, mods only) }
func (h *FlairHandler) Assign(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	slug := r.PathValue("slug")
	c, err := h.communities.GetBySlug(r.Context(), slug)
	if err != nil {
		api.Error(w, http.StatusNotFound, "community not found")
		return
	}
	var req struct {
		FlairID       string `json:"flair_id"`
		ParticipantID string `json:"participant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FlairID == "" {
		api.Error(w, http.StatusBadRequest, "flair_id is required")
		return
	}

	// Default: assign to self. Mods can pass participant_id to assign to anyone.
	targetID := claims.ParticipantID
	if req.ParticipantID != "" && req.ParticipantID != claims.ParticipantID {
		if !h.isMod(r, c.ID, claims.ParticipantID, c.CreatedBy) {
			api.Error(w, http.StatusForbidden, "only mods can assign flairs to other users")
			return
		}
		targetID = req.ParticipantID
	}

	if err := h.flairs.AssignFlair(r.Context(), targetID, c.ID, req.FlairID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to assign flair")
		return
	}
	flair, err := h.flairs.GetFlair(r.Context(), targetID, c.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "flair not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to fetch flair")
		return
	}
	api.JSON(w, http.StatusOK, flair)
}

// Remove handles DELETE /api/v1/communities/{slug}/flair — remove own flair (or a user's flair, mod-only).
func (h *FlairHandler) Remove(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	slug := r.PathValue("slug")
	c, err := h.communities.GetBySlug(r.Context(), slug)
	if err != nil {
		api.Error(w, http.StatusNotFound, "community not found")
		return
	}
	targetID := claims.ParticipantID
	if pid := r.URL.Query().Get("participant_id"); pid != "" && pid != claims.ParticipantID {
		if !h.isMod(r, c.ID, claims.ParticipantID, c.CreatedBy) {
			api.Error(w, http.StatusForbidden, "only mods can remove flairs from other users")
			return
		}
		targetID = pid
	}
	if err := h.flairs.RemoveFlair(r.Context(), targetID, c.ID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to remove flair")
		return
	}
	api.JSON(w, http.StatusNoContent, nil)
}
