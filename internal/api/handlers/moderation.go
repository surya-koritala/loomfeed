package handlers

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// ModerationHandler handles community moderation endpoints.
type ModerationHandler struct {
	moderation   *repository.ModerationRepo
	communities  *repository.CommunityRepo
	reports      *repository.ReportRepo
	qualityGates *repository.QualityGateRepo
	cfg          *config.Config
}

// WithQualityGates exposes the schema-backed policy through the existing
// community settings endpoint and moderation dashboard.
func (h *ModerationHandler) WithQualityGates(gates *repository.QualityGateRepo) {
	h.qualityGates = gates
}

// NewModerationHandler creates a new ModerationHandler.
func NewModerationHandler(
	moderation *repository.ModerationRepo,
	communities *repository.CommunityRepo,
	reports *repository.ReportRepo,
	cfg *config.Config,
) *ModerationHandler {
	return &ModerationHandler{
		moderation:  moderation,
		communities: communities,
		reports:     reports,
		cfg:         cfg,
	}
}

// Dashboard handles GET /api/v1/communities/{slug}/moderation.
// Returns moderator list and pending reports for the community.
func (h *ModerationHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	slug := r.PathValue("slug")
	if slug == "" {
		api.Error(w, http.StatusBadRequest, "slug is required")
		return
	}

	community, err := h.communities.GetBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "community not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to get community")
		return
	}

	// Check caller is creator or moderator
	isMod, err := h.moderation.IsModerator(r.Context(), community.ID, claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to check moderator status")
		return
	}
	if community.CreatedBy != claims.ParticipantID && !isMod {
		api.Error(w, http.StatusForbidden, "not authorized to view moderation dashboard")
		return
	}

	mods, err := h.moderation.ListModerators(r.Context(), community.ID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list moderators")
		return
	}
	if mods == nil {
		mods = []map[string]any{}
	}

	pendingReports, err := h.moderation.GetPendingReports(r.Context(), community.ID, 50)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to get pending reports")
		return
	}
	if pendingReports == nil {
		pendingReports = []map[string]any{}
	}

	qualityGate := &models.QualityGate{CommunityID: community.ID}
	if h.qualityGates != nil {
		gate, gateErr := h.qualityGates.GetByCommunityID(r.Context(), community.ID)
		if gateErr == nil {
			qualityGate = gate
		} else if !errors.Is(gateErr, pgx.ErrNoRows) {
			api.Error(w, http.StatusInternalServerError, "failed to get quality gate")
			return
		}
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"community":       community,
		"quality_gate":    qualityGate,
		"moderators":      mods,
		"pending_reports": pendingReports,
	})
}

// PublicList handles GET /api/v1/communities/{slug}/moderators.
// Public read-only listing of a community's moderators — used by the
// right-rail Moderators card on the community detail page. No auth
// required; response only includes display fields (id, display_name,
// type, trust_score, role, created_at), no email or other sensitive
// data. Authenticated mod-management still goes through the
// /moderation dashboard endpoint above.
func (h *ModerationHandler) PublicList(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		api.Error(w, http.StatusBadRequest, "slug is required")
		return
	}

	community, err := h.communities.GetBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "community not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to get community")
		return
	}

	mods, err := h.moderation.ListModerators(r.Context(), community.ID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list moderators")
		return
	}
	if mods == nil {
		mods = []map[string]any{}
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"moderators": mods,
	})
}

// AddModerator handles POST /api/v1/communities/{slug}/moderators.
// Body: { participant_id, role }
func (h *ModerationHandler) AddModerator(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	slug := r.PathValue("slug")
	if slug == "" {
		api.Error(w, http.StatusBadRequest, "slug is required")
		return
	}

	community, err := h.communities.GetBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "community not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to get community")
		return
	}

	// Only creator or existing admin mods can add moderators
	isMod, err := h.moderation.IsModerator(r.Context(), community.ID, claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to check moderator status")
		return
	}
	if community.CreatedBy != claims.ParticipantID && !isMod {
		api.Error(w, http.StatusForbidden, "not authorized to add moderators")
		return
	}

	var req struct {
		ParticipantID string `json:"participant_id"`
		Role          string `json:"role"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ParticipantID == "" {
		api.Error(w, http.StatusBadRequest, "participant_id is required")
		return
	}
	if req.Role == "" {
		req.Role = "moderator"
	}

	if err := h.moderation.AddModerator(r.Context(), community.ID, req.ParticipantID, req.Role); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to add moderator")
		return
	}

	api.JSON(w, http.StatusCreated, map[string]string{"status": "added"})
}

// RemoveModerator handles DELETE /api/v1/communities/{slug}/moderators/{id}.
func (h *ModerationHandler) RemoveModerator(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	slug := r.PathValue("slug")
	modID := r.PathValue("id")
	if slug == "" || modID == "" {
		api.Error(w, http.StatusBadRequest, "slug and moderator id are required")
		return
	}

	community, err := h.communities.GetBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "community not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to get community")
		return
	}

	// Only creator or existing moderators can remove
	isMod, err := h.moderation.IsModerator(r.Context(), community.ID, claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to check moderator status")
		return
	}
	if community.CreatedBy != claims.ParticipantID && !isMod {
		api.Error(w, http.StatusForbidden, "not authorized to remove moderators")
		return
	}

	// Protect creator from removal
	if community.CreatedBy == modID {
		api.Error(w, http.StatusForbidden, "cannot remove the community creator")
		return
	}

	if err := h.moderation.RemoveModerator(r.Context(), community.ID, modID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to remove moderator")
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// GetMyRole returns the current user's role in a community (creator/admin/moderator/member/none).
// Public endpoint — returns "none" for unauthenticated requests.
func (h *ModerationHandler) GetMyRole(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	claims := middleware.GetClaims(r.Context())

	if claims == nil {
		api.JSON(w, http.StatusOK, map[string]string{"role": "none"})
		return
	}

	community, err := h.communities.GetBySlug(r.Context(), slug)
	if err != nil {
		api.Error(w, http.StatusNotFound, "community not found")
		return
	}

	// Check if creator
	if community.CreatedBy == claims.ParticipantID {
		api.JSON(w, http.StatusOK, map[string]string{"role": "creator"})
		return
	}

	// Check moderator role
	isMod, _ := h.moderation.IsModerator(r.Context(), community.ID, claims.ParticipantID)
	if isMod {
		// Get specific role (admin vs moderator)
		mods, _ := h.moderation.ListModerators(r.Context(), community.ID)
		for _, m := range mods {
			if m["id"] == claims.ParticipantID {
				api.JSON(w, http.StatusOK, map[string]string{"role": m["role"].(string)})
				return
			}
		}
		api.JSON(w, http.StatusOK, map[string]string{"role": "moderator"})
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{"role": "member"})
}

// UpdateSettings handles PUT /api/v1/communities/{slug}/settings.
// Allows creator or admin to update community settings.
func (h *ModerationHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	community, err := h.communities.GetBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "community not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to get community")
		return
	}

	// Only creator or admin moderator can update settings
	isCreator := community.CreatedBy == claims.ParticipantID
	if !isCreator {
		isAdmin := false
		isMod, _ := h.moderation.IsModerator(r.Context(), community.ID, claims.ParticipantID)
		if isMod {
			mods, _ := h.moderation.ListModerators(r.Context(), community.ID)
			for _, m := range mods {
				if m["id"] == claims.ParticipantID && m["role"] == "admin" {
					isAdmin = true
					break
				}
			}
		}
		if !isAdmin {
			api.Error(w, http.StatusForbidden, "only creator or admin can update settings")
			return
		}
	}

	var req struct {
		Description      *string             `json:"description"`
		Rules            *string             `json:"rules"`
		AgentPolicy      *string             `json:"agent_policy"`
		QualityThreshold *float64            `json:"quality_threshold"`
		AllowedPostTypes []string            `json:"allowed_post_types"`
		RequireTags      *bool               `json:"require_tags"`
		MinBodyLength    *int                `json:"min_body_length"`
		QualityGate      *models.QualityGate `json:"quality_gate"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request")
		return
	}

	updates := map[string]any{}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Rules != nil {
		updates["rules"] = *req.Rules
	}
	if req.AgentPolicy != nil {
		updates["agent_policy"] = *req.AgentPolicy
	}
	if req.QualityThreshold != nil {
		if *req.QualityThreshold < 0 || *req.QualityThreshold > 100 {
			api.Error(w, http.StatusBadRequest, "quality_threshold must be between 0 and 100")
			return
		}
		updates["quality_threshold"] = *req.QualityThreshold
	}
	if req.RequireTags != nil {
		updates["require_tags"] = *req.RequireTags
	}
	if req.MinBodyLength != nil {
		updates["min_body_length"] = *req.MinBodyLength
	}

	if req.QualityGate != nil {
		if req.QualityGate.MinTrustScore < 0 || req.QualityGate.MinTrustScore > 100 {
			api.Error(w, http.StatusBadRequest, "quality_gate.min_trust_score must be between 0 and 100")
			return
		}
		if req.QualityGate.MinConfidenceScore < 0 || req.QualityGate.MinConfidenceScore > 1 {
			api.Error(w, http.StatusBadRequest, "quality_gate.min_confidence_score must be between 0 and 1")
			return
		}
		if req.QualityGate.MaxAgentPostsPerHour < 0 || req.QualityGate.MaxAgentPostsPerHour > 10000 {
			api.Error(w, http.StatusBadRequest, "quality_gate.max_agent_posts_per_hour must be between 0 and 10000")
			return
		}
		if h.qualityGates == nil {
			api.Error(w, http.StatusServiceUnavailable, "quality gates are not configured")
			return
		}
		req.QualityGate.CommunityID = community.ID
	}

	if len(updates) == 0 && req.QualityGate == nil {
		api.JSON(w, http.StatusOK, map[string]string{"status": "no changes"})
		return
	}

	if len(updates) > 0 {
		if err := h.communities.UpdateSettings(r.Context(), community.ID, updates); err != nil {
			api.Error(w, http.StatusInternalServerError, "failed to update settings")
			return
		}
	}
	if req.QualityGate != nil {
		if _, err := h.qualityGates.Upsert(r.Context(), req.QualityGate); err != nil {
			api.Error(w, http.StatusInternalServerError, "failed to update quality gate")
			return
		}
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
