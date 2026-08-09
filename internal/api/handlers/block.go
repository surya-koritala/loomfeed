package handlers

import (
	"net/http"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// BlockHandler handles participant_blocks endpoints.
type BlockHandler struct {
	blocks *repository.BlockRepo
}

// NewBlockHandler creates a BlockHandler.
func NewBlockHandler(blocks *repository.BlockRepo) *BlockHandler {
	return &BlockHandler{blocks: blocks}
}

// blockRequest is the payload for POST /api/v1/blocks.
type blockRequest struct {
	ParticipantID string `json:"participant_id"`
}

// Block handles POST /api/v1/blocks. Idempotent — re-blocking the
// same participant returns 200, not 409, so clients don't have to
// special-case the "already blocked" path.
func (h *BlockHandler) Block(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req blockRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ParticipantID == "" {
		api.Error(w, http.StatusBadRequest, "participant_id is required")
		return
	}
	if req.ParticipantID == claims.ParticipantID {
		api.Error(w, http.StatusBadRequest, "cannot block yourself")
		return
	}
	if err := h.blocks.Block(r.Context(), claims.ParticipantID, req.ParticipantID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to block")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"blocked": true})
}

// Unblock handles DELETE /api/v1/blocks/{id}.
func (h *BlockHandler) Unblock(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}
	if err := h.blocks.Unblock(r.Context(), claims.ParticipantID, id); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to unblock")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"blocked": false})
}

// List handles GET /api/v1/blocks. Returns the viewer's blocks
// joined with profile fields so /settings/blocks can render
// avatars + names without N+1.
func (h *BlockHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	rows, err := h.blocks.ListBlockedWithDetails(r.Context(), claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list blocks")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{
		"blocks": rows,
		"total":  len(rows),
	})
}
