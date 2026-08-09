package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// CommunityNoteHandler exposes crowd-verified fact-checks on posts.
type CommunityNoteHandler struct {
	notes *repository.CommunityNoteRepo
}

func NewCommunityNoteHandler(notes *repository.CommunityNoteRepo) *CommunityNoteHandler {
	return &CommunityNoteHandler{notes: notes}
}

type createNoteRequest struct {
	Body    string   `json:"body"`
	Sources []string `json:"sources"`
}

// Create handles POST /api/v1/posts/{id}/notes.
// Any authed participant can add a note. Body 10..1000 chars, at least
// one source URL required.
func (h *CommunityNoteHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req createNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	body := strings.TrimSpace(req.Body)
	if len(body) < 10 || len(body) > 1000 {
		api.Error(w, http.StatusBadRequest, "body must be 10–1000 characters")
		return
	}

	sources := make([]string, 0, len(req.Sources))
	for _, s := range req.Sources {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		// Minimal URL sanity check — must start with http(s)://. More
		// thorough validation (reachability, content-type) is not
		// worth the latency hit on every note create.
		if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
			api.Error(w, http.StatusBadRequest, "sources must be http(s) URLs")
			return
		}
		sources = append(sources, s)
	}
	if len(sources) == 0 {
		api.Error(w, http.StatusBadRequest, "at least one source URL is required")
		return
	}
	if len(sources) > 5 {
		sources = sources[:5]
	}

	note, err := h.notes.Create(r.Context(), postID, claims.ParticipantID, body, sources)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to create note")
		return
	}
	api.JSON(w, http.StatusCreated, note)
}

// List handles GET /api/v1/posts/{id}/notes.
// Public. Returns notes in stable order (shown → pending → hidden,
// newest first within each group). If the caller is authed, each note
// includes the caller's own rating (or null).
func (h *CommunityNoteHandler) List(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}

	viewerID := ""
	if claims := middleware.GetClaims(r.Context()); claims != nil {
		viewerID = claims.ParticipantID
	}

	notes, err := h.notes.ListByPost(r.Context(), postID, viewerID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list notes")
		return
	}
	if notes == nil {
		notes = []repository.CommunityNote{}
	}
	api.JSON(w, http.StatusOK, map[string]any{"notes": notes})
}

type rateNoteRequest struct {
	Rating string `json:"rating"` // helpful | not_helpful
}

// Rate handles POST /api/v1/notes/{id}/rate.
// One rating per rater per note; re-rating overwrites. Note author
// cannot rate their own note — would game the threshold.
func (h *CommunityNoteHandler) Rate(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	noteID := r.PathValue("id")
	if noteID == "" {
		api.Error(w, http.StatusBadRequest, "note id is required")
		return
	}

	var req rateNoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Rating != "helpful" && req.Rating != "not_helpful" {
		api.Error(w, http.StatusBadRequest, "rating must be 'helpful' or 'not_helpful'")
		return
	}

	authorID, err := h.notes.GetAuthor(r.Context(), noteID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "note not found")
		return
	}
	if authorID == claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "you can't rate your own note")
		return
	}

	updated, err := h.notes.Rate(r.Context(), noteID, claims.ParticipantID, req.Rating)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to record rating")
		return
	}
	api.JSON(w, http.StatusOK, updated)
}
