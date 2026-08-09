package handlers

import (
	"net/http"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// CommentBookmarkHandler handles comment bookmark endpoints.
type CommentBookmarkHandler struct {
	commentBookmarks *repository.CommentBookmarkRepo
}

// NewCommentBookmarkHandler creates a new CommentBookmarkHandler.
func NewCommentBookmarkHandler(commentBookmarks *repository.CommentBookmarkRepo) *CommentBookmarkHandler {
	return &CommentBookmarkHandler{commentBookmarks: commentBookmarks}
}

// Toggle handles POST /api/v1/comments/{id}/bookmark.
func (h *CommentBookmarkHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	commentID := r.PathValue("id")
	if commentID == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}
	added, err := h.commentBookmarks.Toggle(r.Context(), claims.ParticipantID, commentID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to toggle comment bookmark")
		return
	}
	api.JSON(w, http.StatusOK, map[string]bool{"bookmarked": added})
}

// List handles GET /api/v1/bookmarks/comments. Returns full comment
// records (body, author, parent post title) in one round trip so
// the bookmarks page can render without an N+1 fetch loop. The
// legacy comment_ids field is kept for any caller still on the old
// shape.
func (h *CommentBookmarkHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	limit := parseIntQuery(r, "limit", 25)
	offset := parseIntQuery(r, "offset", 0)
	comments, total, err := h.commentBookmarks.ListByParticipantWithDetails(r.Context(), claims.ParticipantID, limit, offset)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list comment bookmarks")
		return
	}
	ids := make([]string, len(comments))
	for i, c := range comments {
		ids[i] = c.ID
	}
	api.JSON(w, http.StatusOK, map[string]any{
		"comments":    comments,
		"comment_ids": ids,
		"total":       total,
	})
}
