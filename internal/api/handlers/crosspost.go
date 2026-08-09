package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/models"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// CrosspostHandler handles cross-posting between communities.
type CrosspostHandler struct {
	posts *repository.PostRepo
	cfg   *config.Config
}

// NewCrosspostHandler creates a new CrosspostHandler.
func NewCrosspostHandler(posts *repository.PostRepo, cfg *config.Config) *CrosspostHandler {
	return &CrosspostHandler{posts: posts, cfg: cfg}
}

// Crosspost handles POST /api/v1/posts/{id}/crosspost.
// Body: { "community_id": "<target community UUID>" }
func (h *CrosspostHandler) Crosspost(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	originalID := r.PathValue("id")
	if originalID == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	var req struct {
		CommunityID string `json:"community_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CommunityID == "" {
		api.Error(w, http.StatusBadRequest, "community_id is required")
		return
	}

	// Get the original post
	original, err := h.posts.GetByID(r.Context(), originalID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "post not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to get post")
		return
	}

	// Create the new post in the target community
	newPost := &models.Post{
		CommunityID:     req.CommunityID,
		AuthorID:        claims.ParticipantID,
		AuthorType:      models.ParticipantType(claims.ParticipantType),
		Title:           original.Title,
		Body:            original.Body,
		URL:             original.URL,
		PostType:        original.PostType,
		Metadata:        original.Metadata,
		Tags:            original.Tags,
		ConfidenceScore: original.ConfidenceScore,
		CrosspostedFrom: &originalID,
	}

	result, err := h.posts.CreateWithCrosspost(r.Context(), newPost)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, fmt.Sprintf("failed to create crosspost: %v", err))
		return
	}

	api.JSON(w, http.StatusCreated, result)
}
