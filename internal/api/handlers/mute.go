package handlers

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// MuteHandler handles community_mutes endpoints.
type MuteHandler struct {
	mutes       *repository.MuteRepo
	communities *repository.CommunityRepo
}

// NewMuteHandler creates a MuteHandler.
func NewMuteHandler(mutes *repository.MuteRepo, communities *repository.CommunityRepo) *MuteHandler {
	return &MuteHandler{mutes: mutes, communities: communities}
}

// resolveCommunity accepts either a UUID or a slug and returns the
// community ID. Lets the API surface stay consistent — callers that
// have either form work.
func (h *MuteHandler) resolveCommunity(r *http.Request, ref string) (string, error) {
	// UUID format quick check: 36 chars with hyphens at the right
	// indices. If it doesn't match, treat as a slug.
	if len(ref) == 36 && ref[8] == '-' && ref[13] == '-' && ref[18] == '-' && ref[23] == '-' {
		return ref, nil
	}
	c, err := h.communities.GetBySlug(r.Context(), ref)
	if err != nil {
		return "", err
	}
	return c.ID, nil
}

// muteRequest is the payload for POST /api/v1/mutes.
type muteRequest struct {
	CommunityID   string `json:"community_id"`
	CommunitySlug string `json:"community_slug"`
}

// Mute handles POST /api/v1/mutes. Body accepts either
// community_id or community_slug.
func (h *MuteHandler) Mute(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req muteRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ref := req.CommunityID
	if ref == "" {
		ref = req.CommunitySlug
	}
	if ref == "" {
		api.Error(w, http.StatusBadRequest, "community_id or community_slug is required")
		return
	}
	cid, err := h.resolveCommunity(r, ref)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "community not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to resolve community")
		return
	}
	if err := h.mutes.Mute(r.Context(), claims.ParticipantID, cid); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to mute")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"muted": true})
}

// Unmute handles DELETE /api/v1/mutes/{ref} where ref is either a
// community UUID or slug.
func (h *MuteHandler) Unmute(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	ref := r.PathValue("ref")
	if ref == "" {
		api.Error(w, http.StatusBadRequest, "community ref is required")
		return
	}
	cid, err := h.resolveCommunity(r, ref)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "community not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to resolve community")
		return
	}
	if err := h.mutes.Unmute(r.Context(), claims.ParticipantID, cid); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to unmute")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"muted": false})
}

// List handles GET /api/v1/mutes.
func (h *MuteHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	rows, err := h.mutes.ListMutedWithDetails(r.Context(), claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list mutes")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{
		"mutes": rows,
		"total": len(rows),
	})
}
