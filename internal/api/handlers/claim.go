package handlers

import (
	"net/http"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

type ClaimHandler struct {
	claims *repository.ClaimRepo
}

func NewClaimHandler(claims *repository.ClaimRepo) *ClaimHandler {
	return &ClaimHandler{claims: claims}
}

type createClaimRequest struct {
	ClaimText string  `json:"claim_text"`
	Status    string  `json:"status"`
	Evidence  *string `json:"evidence,omitempty"`
}

func (h *ClaimHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	commentID := r.PathValue("id")
	if commentID == "" {
		api.Error(w, http.StatusBadRequest, "comment id is required")
		return
	}

	var req createClaimRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ClaimText == "" {
		api.Error(w, http.StatusBadRequest, "claim_text is required")
		return
	}
	if req.Status != "verified" && req.Status != "disputed" {
		api.Error(w, http.StatusBadRequest, "status must be 'verified' or 'disputed'")
		return
	}

	cv, err := h.claims.Create(r.Context(), commentID, claims.ParticipantID, req.ClaimText, req.Status, req.Evidence)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to create claim verification")
		return
	}

	api.JSON(w, http.StatusCreated, cv)
}

func (h *ClaimHandler) List(w http.ResponseWriter, r *http.Request) {
	commentID := r.PathValue("id")
	if commentID == "" {
		api.Error(w, http.StatusBadRequest, "comment id is required")
		return
	}

	cvs, err := h.claims.ListByComment(r.Context(), commentID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list claims")
		return
	}
	if cvs == nil {
		cvs = []repository.ClaimVerificationWithVerifier{}
	}

	api.JSON(w, http.StatusOK, cvs)
}
