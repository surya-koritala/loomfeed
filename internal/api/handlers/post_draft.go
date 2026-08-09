package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// PostDraftHandler exposes CRUD over the user's post drafts. All
// routes are JWT-only; drafts are strictly per-owner.
type PostDraftHandler struct {
	drafts *repository.PostDraftRepo
}

func NewPostDraftHandler(drafts *repository.PostDraftRepo) *PostDraftHandler {
	return &PostDraftHandler{drafts: drafts}
}

var validDraftTypes = map[string]bool{
	"text": true, "image": true, "link": true, "poll": true,
}

type draftBody struct {
	CommunityID *string        `json:"community_id"`
	PostType    string         `json:"post_type"`
	Title       string         `json:"title"`
	Body        string         `json:"body"`
	URL         string         `json:"url"`
	Tags        []string       `json:"tags"`
	Metadata    map[string]any `json:"metadata"`
}

func toInput(b *draftBody) repository.PostDraftInput {
	if b.PostType == "" || !validDraftTypes[b.PostType] {
		b.PostType = "text"
	}
	if b.Tags == nil {
		b.Tags = []string{}
	}
	if b.Metadata == nil {
		b.Metadata = map[string]any{}
	}
	// Empty-string community_id from the client means "no community
	// picked yet" — store as NULL.
	if b.CommunityID != nil && *b.CommunityID == "" {
		b.CommunityID = nil
	}
	return repository.PostDraftInput{
		CommunityID: b.CommunityID,
		PostType:    b.PostType,
		Title:       b.Title,
		Body:        b.Body,
		URL:         b.URL,
		Tags:        b.Tags,
		Metadata:    b.Metadata,
	}
}

// List handles GET /api/v1/me/drafts.
func (h *PostDraftHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	drafts, err := h.drafts.ListByOwner(r.Context(), claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list drafts")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"drafts": drafts})
}

// Get handles GET /api/v1/me/drafts/{id}.
func (h *PostDraftHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id required")
		return
	}
	d, err := h.drafts.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "draft not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if d.OwnerID != claims.ParticipantID {
		api.Error(w, http.StatusNotFound, "draft not found")
		return
	}
	api.JSON(w, http.StatusOK, d)
}

// Create handles POST /api/v1/me/drafts.
func (h *PostDraftHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var body draftBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	d, err := h.drafts.Create(r.Context(), claims.ParticipantID, toInput(&body))
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to create draft")
		return
	}
	api.JSON(w, http.StatusCreated, d)
}

// Update handles PUT /api/v1/me/drafts/{id}.
func (h *PostDraftHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id required")
		return
	}
	owner, err := h.drafts.OwnerOf(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "draft not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if owner != claims.ParticipantID {
		api.Error(w, http.StatusNotFound, "draft not found")
		return
	}
	var body draftBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.drafts.Update(r.Context(), id, toInput(&body)); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to update draft")
		return
	}
	d, err := h.drafts.Get(r.Context(), id)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	api.JSON(w, http.StatusOK, d)
}

// Delete handles DELETE /api/v1/me/drafts/{id}.
func (h *PostDraftHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id required")
		return
	}
	owner, err := h.drafts.OwnerOf(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "draft not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if owner != claims.ParticipantID {
		api.Error(w, http.StatusNotFound, "draft not found")
		return
	}
	if err := h.drafts.Delete(r.Context(), id); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to delete draft")
		return
	}
	api.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
