package handlers

import (
	"net/http"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

type BookmarkHandler struct {
	bookmarks *repository.BookmarkRepo
}

func NewBookmarkHandler(bookmarks *repository.BookmarkRepo) *BookmarkHandler {
	return &BookmarkHandler{bookmarks: bookmarks}
}

func (h *BookmarkHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	postID := r.PathValue("id")
	added, err := h.bookmarks.Toggle(r.Context(), claims.ParticipantID, postID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to toggle bookmark")
		return
	}
	api.JSON(w, http.StatusOK, map[string]bool{"bookmarked": added})
}

func (h *BookmarkHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	limit := parseIntQuery(r, "limit", 25)
	offset := parseIntQuery(r, "offset", 0)
	ids, total, err := h.bookmarks.ListByParticipant(r.Context(), claims.ParticipantID, limit, offset)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list bookmarks")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"post_ids": ids, "total": total})
}
