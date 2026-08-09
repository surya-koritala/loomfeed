package handlers

import (
	"net/http"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// MentionHandler handles mention endpoints.
type MentionHandler struct {
	participants *repository.ParticipantRepo
	mentions     *repository.MentionRepo
}

// NewMentionHandler creates a new MentionHandler.
func NewMentionHandler(participants *repository.ParticipantRepo) *MentionHandler {
	return &MentionHandler{participants: participants}
}

// WithMentions wires the mention repo so the inbox endpoint
// (GET /api/v1/profiles/me/mentions) can list places the
// authenticated participant has been mentioned.
func (h *MentionHandler) WithMentions(m *repository.MentionRepo) {
	h.mentions = m
}

// MyMentions handles GET /api/v1/profiles/me/mentions. Returns
// posts and comments that mention the authenticated participant,
// newest first. Backs the Mentions tab on the user's profile.
func (h *MentionHandler) MyMentions(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	if h.mentions == nil {
		api.Error(w, http.StatusServiceUnavailable, "mentions not configured")
		return
	}
	limit := parseIntQuery(r, "limit", 25)
	if limit > 100 {
		limit = 100
	}
	offset := parseIntQuery(r, "offset", 0)

	items, total, err := h.mentions.ListForRecipient(r.Context(), claims.ParticipantID, limit, offset)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list mentions")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{
		"mentions": items,
		"total":    total,
	})
}

// Autocomplete handles GET /api/v1/mentions/autocomplete?q=prefix — returns
// matching participants by display name prefix for @mention suggestions.
func (h *MentionHandler) Autocomplete(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("q")
	if prefix == "" {
		api.JSON(w, http.StatusOK, []any{})
		return
	}

	limit := parseIntQuery(r, "limit", 10)
	if limit > 25 {
		limit = 25
	}

	participants, err := h.participants.SearchByDisplayNamePrefix(r.Context(), prefix, limit)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to search participants")
		return
	}

	// Return a lightweight response for autocomplete
	type suggestion struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		AvatarURL   string `json:"avatar_url,omitempty"`
		Type        string `json:"type"`
		IsVerified  bool   `json:"is_verified"`
	}

	results := make([]suggestion, 0, len(participants))
	for _, p := range participants {
		results = append(results, suggestion{
			ID:          p.ID,
			DisplayName: p.DisplayName,
			AvatarURL:   p.AvatarURL,
			Type:        string(p.Type),
			IsVerified:  p.IsVerified,
		})
	}

	api.JSON(w, http.StatusOK, results)
}
