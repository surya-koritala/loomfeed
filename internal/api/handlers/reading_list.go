package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// ReadingListHandler exposes reading-list CRUD + item add/remove.
// Visibility: lists are public-by-default; private lists return 404
// to non-owners (rather than 403 — don't leak the list's existence).
type ReadingListHandler struct {
	lists *repository.ReadingListRepo
}

func NewReadingListHandler(lists *repository.ReadingListRepo) *ReadingListHandler {
	return &ReadingListHandler{lists: lists}
}

type createListRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	IsPublic    *bool  `json:"is_public,omitempty"`
}

type updateListRequest struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	IsPublic    *bool   `json:"is_public,omitempty"`
}

type addItemRequest struct {
	PostID string `json:"post_id"`
	Note   string `json:"note"`
}

// Create handles POST /api/v1/reading-lists.
func (h *ReadingListHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req createListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	title := strings.TrimSpace(req.Title)
	if len(title) < 1 || len(title) > 120 {
		api.Error(w, http.StatusBadRequest, "title must be 1–120 characters")
		return
	}
	isPublic := true
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}
	list, err := h.lists.Create(r.Context(), claims.ParticipantID, title, req.Description, isPublic)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to create list")
		return
	}
	api.JSON(w, http.StatusCreated, list)
}

// Get handles GET /api/v1/reading-lists/{id}. Returns the list + its
// items. Private lists are 404 to everyone except the owner.
func (h *ReadingListHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	list, err := h.lists.Get(r.Context(), id)
	if err != nil {
		api.Error(w, http.StatusNotFound, "list not found")
		return
	}

	// Enforce visibility.
	if !list.IsPublic {
		claims := middleware.GetClaims(r.Context())
		if claims == nil || claims.ParticipantID != list.OwnerID {
			api.Error(w, http.StatusNotFound, "list not found")
			return
		}
	}

	items, err := h.lists.Items(r.Context(), id)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to load items")
		return
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"list":  list,
		"items": items,
	})
}

// ListMine handles GET /api/v1/me/reading-lists. Authed only.
func (h *ReadingListHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	out, err := h.lists.ListByOwner(r.Context(), claims.ParticipantID, false)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"lists": out})
}

// ListByOwner handles GET /api/v1/participants/{id}/reading-lists —
// shows the participant's PUBLIC lists (so a profile can surface
// what they've curated). Private lists are never included.
func (h *ReadingListHandler) ListByOwnerPublic(w http.ResponseWriter, r *http.Request) {
	ownerID := r.PathValue("id")
	if ownerID == "" {
		api.Error(w, http.StatusBadRequest, "owner id is required")
		return
	}
	out, err := h.lists.ListByOwner(r.Context(), ownerID, true)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"lists": out})
}

// Update handles PATCH /api/v1/reading-lists/{id}. Owner-only.
func (h *ReadingListHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	id := r.PathValue("id")
	list, err := h.lists.Get(r.Context(), id)
	if err != nil {
		api.Error(w, http.StatusNotFound, "list not found")
		return
	}
	if list.OwnerID != claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "not the owner")
		return
	}
	var req updateListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title != nil {
		t := strings.TrimSpace(*req.Title)
		if len(t) < 1 || len(t) > 120 {
			api.Error(w, http.StatusBadRequest, "title must be 1–120 characters")
			return
		}
		req.Title = &t
	}
	if err := h.lists.Update(r.Context(), id, req.Title, req.Description, req.IsPublic); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to update")
		return
	}
	updated, _ := h.lists.Get(r.Context(), id)
	api.JSON(w, http.StatusOK, updated)
}

// Delete handles DELETE /api/v1/reading-lists/{id}. Owner-only.
func (h *ReadingListHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	id := r.PathValue("id")
	list, err := h.lists.Get(r.Context(), id)
	if err != nil {
		api.Error(w, http.StatusNotFound, "list not found")
		return
	}
	if list.OwnerID != claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "not the owner")
		return
	}
	if err := h.lists.Delete(r.Context(), id); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to delete")
		return
	}
	api.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// AddItem handles POST /api/v1/reading-lists/{id}/items. Owner-only.
func (h *ReadingListHandler) AddItem(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	id := r.PathValue("id")
	list, err := h.lists.Get(r.Context(), id)
	if err != nil {
		api.Error(w, http.StatusNotFound, "list not found")
		return
	}
	if list.OwnerID != claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "not the owner")
		return
	}
	var req addItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.PostID == "" {
		api.Error(w, http.StatusBadRequest, "post_id is required")
		return
	}
	if err := h.lists.AddItem(r.Context(), id, req.PostID, req.Note); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to add item")
		return
	}
	api.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// RemoveItem handles DELETE /api/v1/reading-lists/{id}/items/{postId}. Owner-only.
func (h *ReadingListHandler) RemoveItem(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	id := r.PathValue("id")
	postID := r.PathValue("postId")
	list, err := h.lists.Get(r.Context(), id)
	if err != nil {
		api.Error(w, http.StatusNotFound, "list not found")
		return
	}
	if list.OwnerID != claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "not the owner")
		return
	}
	if err := h.lists.RemoveItem(r.Context(), id, postID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to remove item")
		return
	}
	api.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
