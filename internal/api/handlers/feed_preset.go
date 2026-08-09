package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// FeedPresetHandler exposes CRUD for saved feed configurations.
// All routes require JWT — presets are per-user.
type FeedPresetHandler struct {
	presets *repository.FeedPresetRepo
}

func NewFeedPresetHandler(presets *repository.FeedPresetRepo) *FeedPresetHandler {
	return &FeedPresetHandler{presets: presets}
}

var validSorts = map[string]bool{
	"hot": true, "new": true, "top": true, "rising": true, "for-you": true,
}
var validScopes = map[string]bool{
	"global": true, "subscribed": true, "community": true,
}

type presetBody struct {
	Name          string `json:"name"`
	Sort          string `json:"sort"`
	PostType      string `json:"post_type"`
	Scope         string `json:"scope"`
	CommunitySlug string `json:"community_slug"`
}

func normalizePreset(b *presetBody) (string, bool) {
	b.Name = strings.TrimSpace(b.Name)
	if len(b.Name) < 1 || len(b.Name) > 60 {
		return "name must be 1–60 characters", false
	}
	if b.Sort == "" {
		b.Sort = "hot"
	}
	if !validSorts[b.Sort] {
		return "sort must be one of: hot, new, top, rising, for-you", false
	}
	if b.Scope == "" {
		b.Scope = "global"
	}
	if !validScopes[b.Scope] {
		return "scope must be one of: global, subscribed, community", false
	}
	if b.Scope == "community" && strings.TrimSpace(b.CommunitySlug) == "" {
		return "community_slug is required when scope = community", false
	}
	if b.Scope != "community" {
		b.CommunitySlug = ""
	}
	return "", true
}

// List handles GET /api/v1/me/feed-presets.
func (h *FeedPresetHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	presets, err := h.presets.ListByOwner(r.Context(), claims.ParticipantID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list presets")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{"presets": presets})
}

// Create handles POST /api/v1/me/feed-presets.
func (h *FeedPresetHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var body presetBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := normalizePreset(&body); !ok {
		api.Error(w, http.StatusBadRequest, msg)
		return
	}
	p, err := h.presets.Create(r.Context(), claims.ParticipantID, body.Name, body.Sort, body.PostType, body.Scope, body.CommunitySlug)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to create preset")
		return
	}
	api.JSON(w, http.StatusCreated, p)
}

// Update handles PUT /api/v1/me/feed-presets/{id}.
func (h *FeedPresetHandler) Update(w http.ResponseWriter, r *http.Request) {
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
	owner, err := h.presets.OwnerOf(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "preset not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if owner != claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "not your preset")
		return
	}
	var body presetBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := normalizePreset(&body); !ok {
		api.Error(w, http.StatusBadRequest, msg)
		return
	}
	if err := h.presets.Update(r.Context(), id, body.Name, body.Sort, body.PostType, body.Scope, body.CommunitySlug); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to update preset")
		return
	}
	api.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// Delete handles DELETE /api/v1/me/feed-presets/{id}.
func (h *FeedPresetHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
	owner, err := h.presets.OwnerOf(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "preset not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if owner != claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "not your preset")
		return
	}
	if err := h.presets.Delete(r.Context(), id); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to delete preset")
		return
	}
	api.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
