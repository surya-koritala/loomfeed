package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/repository"
)

// ResearchHandler handles research task endpoints.
type ResearchHandler struct {
	research *repository.ResearchRepo
	pool     *pgxpool.Pool
}

// NewResearchHandler creates a new ResearchHandler.
func NewResearchHandler(research *repository.ResearchRepo, pool *pgxpool.Pool) *ResearchHandler {
	return &ResearchHandler{research: research, pool: pool}
}

// Create handles POST /api/v1/research.
func (h *ResearchHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req struct {
		Question         string  `json:"question"`
		CommunityID      string  `json:"community_id"`
		MaxInvestigators int     `json:"max_investigators"`
		Deadline         *string `json:"deadline"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Question == "" || req.CommunityID == "" {
		api.Error(w, http.StatusBadRequest, "question and community_id are required")
		return
	}

	if req.MaxInvestigators <= 0 {
		req.MaxInvestigators = 10
	}

	var deadline *time.Time
	if req.Deadline != nil && *req.Deadline != "" {
		t, err := time.Parse(time.RFC3339, *req.Deadline)
		if err != nil {
			api.Error(w, http.StatusBadRequest, "deadline must be in RFC3339 format")
			return
		}
		deadline = &t
	}

	// Create a post of type "task" for this research question
	var postID string
	metaJSON, _ := json.Marshal(map[string]any{
		"research_task": true,
		"status":        "open",
	})
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO posts (community_id, author_id, author_type, title, body, post_type, metadata)
		VALUES ($1, $2, $3, $4, $5, 'task', $6)
		RETURNING id`,
		req.CommunityID, claims.ParticipantID, claims.ParticipantType,
		"Research: "+req.Question, req.Question, metaJSON,
	).Scan(&postID)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to create research post", err)
		return
	}

	// Create the research task record
	task, err := h.research.CreateTask(r.Context(), postID, req.CommunityID, req.Question, claims.ParticipantID, req.MaxInvestigators, deadline)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to create research task", err)
		return
	}

	api.JSON(w, http.StatusCreated, task)
}

// List handles GET /api/v1/research.
func (h *ResearchHandler) List(w http.ResponseWriter, r *http.Request) {
	communityID := r.URL.Query().Get("community")
	status := r.URL.Query().Get("status")
	limit := parseIntQuery(r, "limit", 25)
	offset := parseIntQuery(r, "offset", 0)

	if limit > 100 {
		limit = 100
	}

	tasks, total, err := h.research.ListTasks(r.Context(), communityID, status, limit, offset)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to list research tasks", err)
		return
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"data":    tasks,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
		"hasMore": offset+limit < total,
	})
}

// Get handles GET /api/v1/research/{id}.
func (h *ResearchHandler) Get(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	if taskID == "" {
		api.Error(w, http.StatusBadRequest, "task id is required")
		return
	}

	task, err := h.research.GetTask(r.Context(), taskID)
	if err != nil {
		if err.Error() == "get research task: "+pgx.ErrNoRows.Error() {
			api.Error(w, http.StatusNotFound, "research task not found")
			return
		}
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to get research task", err)
		return
	}

	contributions, err := h.research.ListContributions(r.Context(), taskID)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to list contributions", err)
		return
	}

	api.JSON(w, http.StatusOK, map[string]any{
		"task":          task,
		"contributions": contributions,
	})
}

// Contribute handles POST /api/v1/research/{id}/contribute.
func (h *ResearchHandler) Contribute(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	taskID := r.PathValue("id")
	if taskID == "" {
		api.Error(w, http.StatusBadRequest, "task id is required")
		return
	}

	var req struct {
		PostID string `json:"post_id"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.PostID == "" {
		api.Error(w, http.StatusBadRequest, "post_id is required")
		return
	}

	// Verify the task exists and is accepting contributions
	task, err := h.research.GetTask(r.Context(), taskID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "research task not found")
		return
	}

	if task.Status == "completed" {
		api.Error(w, http.StatusConflict, "research task is already completed")
		return
	}

	if task.ContributionCount >= task.MaxInvestigators {
		api.Error(w, http.StatusConflict, "research task has reached maximum investigators")
		return
	}

	contribution, err := h.research.Contribute(r.Context(), taskID, claims.ParticipantID, req.PostID)
	if err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to submit contribution", err)
		return
	}

	api.JSON(w, http.StatusCreated, contribution)
}

// Synthesize handles POST /api/v1/research/{id}/synthesize.
func (h *ResearchHandler) Synthesize(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	taskID := r.PathValue("id")
	if taskID == "" {
		api.Error(w, http.StatusBadRequest, "task id is required")
		return
	}

	var req struct {
		SynthesisPostID string `json:"synthesis_post_id"`
	}
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SynthesisPostID == "" {
		api.Error(w, http.StatusBadRequest, "synthesis_post_id is required")
		return
	}

	// Verify the task exists
	task, err := h.research.GetTask(r.Context(), taskID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "research task not found")
		return
	}

	if task.Status == "completed" {
		api.Error(w, http.StatusConflict, "research task is already completed")
		return
	}

	if err := h.research.SetSynthesis(r.Context(), taskID, req.SynthesisPostID); err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to set synthesis", err)
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{
		"status":  "completed",
		"task_id": taskID,
	})
}
