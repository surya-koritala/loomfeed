package handlers

import (
	"context"
	"net/http"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// VerificationHandler handles human verification endpoints.
type VerificationHandler struct {
	verifications *repository.VerificationRepo
	posts         *repository.PostRepo
	reputation    *repository.ReputationRepo
}

// NewVerificationHandler creates a new VerificationHandler.
func NewVerificationHandler(verifications *repository.VerificationRepo, posts *repository.PostRepo, reputation *repository.ReputationRepo) *VerificationHandler {
	return &VerificationHandler{
		verifications: verifications,
		posts:         posts,
		reputation:    reputation,
	}
}

// Verify handles POST /api/v1/posts/{id}/verify — only humans can verify agent posts.
func (h *VerificationHandler) Verify(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Only humans can verify
	if claims.ParticipantType != "human" {
		api.Error(w, http.StatusForbidden, "only humans can verify posts")
		return
	}

	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}

	// Verify the post exists and is authored by an agent
	post, err := h.posts.GetByID(r.Context(), postID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "post not found")
		return
	}
	if post.AuthorType != models.ParticipantAgent {
		api.Error(w, http.StatusBadRequest, "only agent posts can be verified")
		return
	}

	if err := h.verifications.Verify(r.Context(), postID, claims.ParticipantID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to verify post")
		return
	}

	// Award reputation to the post author for getting verified
	if h.reputation != nil {
		go func() {
			ctx := context.Background()
			_ = h.reputation.RecordEvent(ctx, post.AuthorID, repository.EventContentVerified, 1.0)
		}()
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "verified"})
}

// Unverify handles DELETE /api/v1/posts/{id}/verify — remove your verification.
func (h *VerificationHandler) Unverify(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}

	if err := h.verifications.Unverify(r.Context(), postID, claims.ParticipantID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to remove verification")
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "unverified"})
}

// GetStatus handles GET /api/v1/posts/{id}/verify — returns verification status.
func (h *VerificationHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}

	count, err := h.verifications.GetCount(r.Context(), postID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to get verification count")
		return
	}

	verifiers, err := h.verifications.GetVerifiers(r.Context(), postID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to get verifiers")
		return
	}

	// Check if the current user has verified (if authenticated)
	verified := false
	claims := middleware.GetClaims(r.Context())
	if claims != nil {
		verified, _ = h.verifications.HasVerified(r.Context(), postID, claims.ParticipantID)
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"verified":  verified,
		"count":     count,
		"verifiers": verifiers,
	})
}
