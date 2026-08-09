package handlers

import (
	"net/http"
	"strconv"

	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// CitationHandler handles citation endpoints.
type CitationHandler struct {
	citations *repository.CitationRepo
}

// NewCitationHandler creates a new CitationHandler.
func NewCitationHandler(citations *repository.CitationRepo) *CitationHandler {
	return &CitationHandler{citations: citations}
}

// Create handles POST /api/v1/posts/{id}/citations.
// Body: {"cited_post_id": "...", "citation_type": "supports"}
func (h *CitationHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	sourcePostID := r.PathValue("id")
	if sourcePostID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}

	var req struct {
		CitedPostID  string `json:"cited_post_id"`
		CitationType string `json:"citation_type"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.CitedPostID == "" {
		api.Error(w, http.StatusBadRequest, "cited_post_id is required")
		return
	}

	if sourcePostID == req.CitedPostID {
		api.Error(w, http.StatusBadRequest, "a post cannot cite itself")
		return
	}

	if req.CitationType == "" {
		req.CitationType = "references"
	}

	if err := h.citations.Create(r.Context(), sourcePostID, req.CitedPostID, req.CitationType); err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to create citation", err)
		return
	}

	api.JSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}

// GetByPost handles GET /api/v1/posts/{id}/citations.
func (h *CitationHandler) GetByPost(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}

	citations, err := h.citations.GetByPost(r.Context(), postID)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get citations", err)
		return
	}
	if citations == nil {
		citations = []repository.Citation{}
	}

	api.JSON(w, http.StatusOK, map[string]any{"citations": citations})
}

// GetGraph handles GET /api/v1/posts/{id}/graph?depth=2.
func (h *CitationHandler) GetGraph(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("id")
	if postID == "" {
		api.Error(w, http.StatusBadRequest, "post id is required")
		return
	}

	depth := 2
	if d := r.URL.Query().Get("depth"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed >= 1 && parsed <= 5 {
			depth = parsed
		}
	}

	graph, err := h.citations.GetGraph(r.Context(), postID, depth)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get citation graph", err)
		return
	}

	api.JSON(w, http.StatusOK, graph)
}
