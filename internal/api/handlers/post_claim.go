package handlers

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// PostClaimHandler serves claim-level citation endpoints for posts.
type PostClaimHandler struct {
	claims *repository.PostClaimRepo
	posts  *repository.PostRepo
}

func NewPostClaimHandler(claims *repository.PostClaimRepo, posts *repository.PostRepo) *PostClaimHandler {
	return &PostClaimHandler{claims: claims, posts: posts}
}

// List handles GET /api/v1/posts/{id}/claims.
func (h *PostClaimHandler) List(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}

	claims, err := h.claims.ListByPost(r.Context(), postID)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to list claims", err)
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"claims": claims})
}

// Replace handles PUT /api/v1/posts/{id}/claims.
// Only the post author can replace claims. Body: {"claims": [ {claim_text, citations: [...]}, ... ]}.
func (h *PostClaimHandler) Replace(w http.ResponseWriter, r *http.Request) {
	auth := middleware.GetClaims(r.Context())
	if auth == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}

	post, err := h.posts.GetByID(r.Context(), postID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "post not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to fetch post")
		return
	}
	if post.AuthorID != auth.ParticipantID {
		api.Error(w, http.StatusForbidden, "only the post author can edit claims")
		return
	}

	var req struct {
		Claims []repository.PostClaimInput `json:"claims"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	valid := []string{"supports", "contradicts", "extends", "quotes"}
	for i, c := range req.Claims {
		if strings.TrimSpace(c.ClaimText) == "" {
			api.Error(w, http.StatusBadRequest, "claim_text is required")
			return
		}
		for j, cit := range c.Citations {
			if strings.TrimSpace(cit.SourceURL) == "" {
				api.Error(w, http.StatusBadRequest, "source_url is required on every citation")
				return
			}
			if cit.Relation == "" {
				req.Claims[i].Citations[j].Relation = "supports"
				continue
			}
			if !slices.Contains(valid, cit.Relation) {
				api.Error(w, http.StatusBadRequest, "relation must be one of supports|contradicts|extends|quotes")
				return
			}
		}
	}

	saved, err := h.claims.ReplaceAll(r.Context(), postID, req.Claims)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to save claims", err)
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"claims": saved})
}
