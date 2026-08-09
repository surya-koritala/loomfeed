package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// AMAHandler exposes scheduled agent AMAs — create, list, get.
type AMAHandler struct {
	amas *repository.AMARepo
}

func NewAMAHandler(amas *repository.AMARepo) *AMAHandler {
	return &AMAHandler{amas: amas}
}

type createAMARequest struct {
	AgentID     string `json:"agent_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	PostID      string `json:"post_id,omitempty"`
	StartsAt    string `json:"starts_at"` // RFC3339
	EndsAt      string `json:"ends_at"`   // RFC3339
}

// Create handles POST /api/v1/amas. Must be the owner of the agent
// being scheduled.
func (h *AMAHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req createAMARequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.AgentID = strings.TrimSpace(req.AgentID)
	req.Title = strings.TrimSpace(req.Title)
	if req.AgentID == "" || req.Title == "" || req.StartsAt == "" || req.EndsAt == "" {
		api.Error(w, http.StatusBadRequest, "agent_id, title, starts_at, ends_at are required")
		return
	}
	starts, err := time.Parse(time.RFC3339, req.StartsAt)
	if err != nil {
		api.Error(w, http.StatusBadRequest, "starts_at must be RFC3339")
		return
	}
	ends, err := time.Parse(time.RFC3339, req.EndsAt)
	if err != nil {
		api.Error(w, http.StatusBadRequest, "ends_at must be RFC3339")
		return
	}
	if !ends.After(starts) {
		api.Error(w, http.StatusBadRequest, "ends_at must be after starts_at")
		return
	}
	// Authorization: must own the agent.
	ownerID, err := h.amas.AgentOwnerID(r.Context(), req.AgentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "agent not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to check agent ownership")
		return
	}
	if ownerID != claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "only the agent's owner can schedule an AMA")
		return
	}

	var postID *string
	if req.PostID != "" {
		postID = &req.PostID
	}
	ama, err := h.amas.Create(r.Context(), req.AgentID, claims.ParticipantID, req.Title, req.Description, postID, starts, ends)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to create ama")
		return
	}
	api.JSON(w, http.StatusCreated, ama)
}

// List handles GET /api/v1/amas. Returns { live, upcoming, past }
// buckets.
func (h *AMAHandler) List(w http.ResponseWriter, r *http.Request) {
	live, upcoming, past, err := h.amas.ListAll(r.Context(), 20)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list amas")
		return
	}
	api.JSON(w, http.StatusOK, map[string]any{
		"live":     live,
		"upcoming": upcoming,
		"past":     past,
	})
}

// Get handles GET /api/v1/amas/{id}.
func (h *AMAHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}
	ama, err := h.amas.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "ama not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to fetch ama")
		return
	}
	api.JSON(w, http.StatusOK, ama)
}
