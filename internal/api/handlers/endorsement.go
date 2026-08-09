package handlers

import (
	"context"
	"net/http"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/safego"
)

// EndorsementHandler handles endorsement endpoints.
type EndorsementHandler struct {
	endorsements *repository.EndorsementRepo
	reputation   *repository.ReputationRepo
}

// NewEndorsementHandler creates a new EndorsementHandler.
func NewEndorsementHandler(endorsements *repository.EndorsementRepo, reputation *repository.ReputationRepo) *EndorsementHandler {
	return &EndorsementHandler{
		endorsements: endorsements,
		reputation:   reputation,
	}
}

// Endorse handles POST /api/v1/agents/{id}/endorse.
func (h *EndorsementHandler) Endorse(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	agentID := r.PathValue("id")
	if agentID == "" {
		api.Error(w, http.StatusBadRequest, "agent id is required")
		return
	}

	if agentID == claims.ParticipantID {
		api.Error(w, http.StatusBadRequest, "you cannot endorse yourself")
		return
	}

	var req struct {
		Capability string `json:"capability"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Capability == "" {
		api.Error(w, http.StatusBadRequest, "capability is required")
		return
	}

	if err := h.endorsements.Endorse(r.Context(), claims.ParticipantID, agentID, req.Capability); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to endorse")
		return
	}

	// Award +0.5 reputation to the endorsed agent. Fire-and-forget,
	// hardened with safego: panic recovery + 30s timeout. We don't
	// inherit r.Context() because the request returns 200 before
	// this completes — cancellation would routinely abort the work.
	safego.Run(r.Context(), "endorse-rep", 0, func(ctx context.Context) {
		_ = h.reputation.RecordEvent(ctx, agentID, repository.EventAgentEndorsed, 0.5)
	})

	api.JSON(w, http.StatusOK, map[string]string{"status": "endorsed"})
}

// Unendorse handles DELETE /api/v1/agents/{id}/endorse.
func (h *EndorsementHandler) Unendorse(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	agentID := r.PathValue("id")
	if agentID == "" {
		api.Error(w, http.StatusBadRequest, "agent id is required")
		return
	}

	var req struct {
		Capability string `json:"capability"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Capability == "" {
		api.Error(w, http.StatusBadRequest, "capability is required")
		return
	}

	if err := h.endorsements.Unendorse(r.Context(), claims.ParticipantID, agentID, req.Capability); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to remove endorsement")
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "unendorsed"})
}

// GetEndorsements handles GET /api/v1/agents/{id}/endorsements.
func (h *EndorsementHandler) GetEndorsements(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		api.Error(w, http.StatusBadRequest, "agent id is required")
		return
	}

	counts, err := h.endorsements.CountByCapability(r.Context(), agentID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to get endorsements")
		return
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"agent_id": agentID,
		"counts":   counts,
	})
}
