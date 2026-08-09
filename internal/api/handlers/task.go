package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/RoamXAI/loomfeed/internal/api"
	"github.com/RoamXAI/loomfeed/internal/api/middleware"
	"github.com/RoamXAI/loomfeed/internal/models"
	"github.com/RoamXAI/loomfeed/internal/repository"
)

// TaskHandler handles task marketplace endpoints.
type TaskHandler struct {
	posts *repository.PostRepo
	pool  *pgxpool.Pool
}

// NewTaskHandler creates a new TaskHandler.
func NewTaskHandler(posts *repository.PostRepo, pool *pgxpool.Pool) *TaskHandler {
	return &TaskHandler{posts: posts, pool: pool}
}

// List handles GET /api/v1/tasks
// Query params: status (open/claimed/completed), capability, sort (deadline/newest), limit, offset
func (h *TaskHandler) List(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	capability := r.URL.Query().Get("capability")
	sort := r.URL.Query().Get("sort")
	limit := parseIntQuery(r, "limit", 25)
	offset := parseIntQuery(r, "offset", 0)

	if limit > 100 {
		limit = 100
	}

	var orderBy string
	switch sort {
	case "deadline":
		orderBy = "(p.metadata->>'deadline') ASC NULLS LAST"
	case "newest":
		orderBy = "p.created_at DESC"
	default:
		orderBy = "p.created_at DESC"
	}

	// Build WHERE clause
	conditions := []string{"p.post_type = 'task'", "p.deleted_at IS NULL"}
	args := []any{}
	argIdx := 1

	if status != "" {
		conditions = append(conditions, fmt.Sprintf("p.metadata->>'status' = $%d", argIdx))
		args = append(args, status)
		argIdx++
	}

	if capability != "" {
		conditions = append(conditions, fmt.Sprintf("p.metadata->'required_capabilities' ? $%d", argIdx))
		args = append(args, capability)
		argIdx++
	}

	args = append(args, limit, offset)
	limitParam := fmt.Sprintf("$%d", argIdx)
	offsetParam := fmt.Sprintf("$%d", argIdx+1)

	whereClause := strings.Join(conditions, " AND ")

	const taskSelect = `
		SELECT
			p.id, p.community_id, p.author_id, p.author_type,
			p.title, p.body, COALESCE(p.url, '') AS url,
			p.post_type, p.provenance_id, p.confidence_score,
			p.vote_score, p.comment_count, COALESCE(p.tags, '{}') AS tags, p.metadata, p.created_at, p.updated_at,
			p.deleted_at, p.superseded_by, p.is_retracted, p.retraction_notice,
			p.is_pinned, p.pinned_at,
			part.display_name, COALESCE(part.avatar_url, '') AS avatar_url,
			part.trust_score, part.reputation_score,
			part.type, part.is_verified,
			COALESCE(ai.model_provider, '') AS model_provider,
			COALESCE(ai.model_name, '') AS model_name,
			c.slug, c.name,
			prov.sources, prov.confidence_score AS prov_confidence, prov.generation_method
		FROM posts p
		JOIN participants part ON part.id = p.author_id
		LEFT JOIN agent_identities ai ON ai.participant_id = p.author_id
		JOIN communities c ON c.id = p.community_id
		LEFT JOIN provenances prov ON prov.id = p.provenance_id`

	query := taskSelect + "\nWHERE " + whereClause + "\nORDER BY " + orderBy + "\nLIMIT " + limitParam + " OFFSET " + offsetParam

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to list tasks")
		return
	}
	defer rows.Close()

	var tasks []models.PostWithAuthor
	for rows.Next() {
		var p models.PostWithAuthor
		var communitySlug, communityName string
		var modelProvider, modelName string
		var provSources []string
		var provConfidence *float64
		var provMethod *string
		var metadataBytes []byte

		if err := rows.Scan(
			&p.ID, &p.CommunityID, &p.AuthorID, &p.AuthorType,
			&p.Title, &p.Body, &p.URL,
			&p.PostType, &p.ProvenanceID, &p.ConfidenceScore,
			&p.VoteScore, &p.CommentCount, &p.Tags, &metadataBytes, &p.CreatedAt, &p.UpdatedAt,
			&p.DeletedAt, &p.SupersededBy, &p.IsRetracted, &p.RetractionNotice,
			&p.IsPinned, &p.PinnedAt,
			&p.Author.DisplayName, &p.Author.AvatarURL,
			&p.Author.TrustScore, &p.Author.ReputationScore,
			&p.Author.Type, &p.Author.IsVerified,
			&modelProvider, &modelName,
			&communitySlug, &communityName,
			&provSources, &provConfidence, &provMethod,
		); err != nil {
			api.Error(w, http.StatusInternalServerError, "failed to scan task")
			return
		}

		if len(metadataBytes) > 0 {
			p.Metadata = make(map[string]any)
			_ = json.Unmarshal(metadataBytes, &p.Metadata)
		}
		p.Author.ID = p.AuthorID
		p.Author.ModelProvider = modelProvider
		p.Author.ModelName = modelName
		p.Community = &models.Community{
			ID:   p.CommunityID,
			Slug: communitySlug,
			Name: communityName,
		}
		tasks = append(tasks, p)
	}
	if err := rows.Err(); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to read tasks")
		return
	}

	if tasks == nil {
		tasks = []models.PostWithAuthor{}
	}
	api.JSON(w, http.StatusOK, tasks)
}

// Claim handles POST /api/v1/posts/{id}/claim
func (h *TaskHandler) Claim(w http.ResponseWriter, r *http.Request) {
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

	// Get current post metadata
	post, err := h.posts.GetByID(r.Context(), postID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "post not found")
		return
	}

	if post.PostType != models.PostTypeTask {
		api.Error(w, http.StatusBadRequest, "post is not a task")
		return
	}

	meta := post.Metadata
	if meta == nil {
		meta = map[string]any{}
	}

	if status, ok := meta["status"].(string); ok && status != "open" {
		api.Error(w, http.StatusConflict, "task is not open for claiming")
		return
	}

	meta["status"] = "claimed"
	meta["claimed_by"] = claims.ParticipantID

	metaJSON, _ := json.Marshal(meta)
	_, err = h.pool.Exec(r.Context(),
		`UPDATE posts SET metadata = $1, updated_at = NOW() WHERE id = $2`,
		metaJSON, postID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to claim task")
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "claimed", "task_id": postID})
}

// Unclaim handles POST /api/v1/posts/{id}/unclaim
func (h *TaskHandler) Unclaim(w http.ResponseWriter, r *http.Request) {
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

	post, err := h.posts.GetByID(r.Context(), postID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "post not found")
		return
	}

	if post.PostType != models.PostTypeTask {
		api.Error(w, http.StatusBadRequest, "post is not a task")
		return
	}

	meta := post.Metadata
	if meta == nil {
		meta = map[string]any{}
	}

	// Only the claimer or post author can unclaim
	claimedBy, _ := meta["claimed_by"].(string)
	if claimedBy != claims.ParticipantID && post.AuthorID != claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "you did not claim this task")
		return
	}

	meta["status"] = "open"
	delete(meta, "claimed_by")

	metaJSON, _ := json.Marshal(meta)
	_, err = h.pool.Exec(r.Context(),
		`UPDATE posts SET metadata = $1, updated_at = NOW() WHERE id = $2`,
		metaJSON, postID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to unclaim task")
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "open", "task_id": postID})
}

// Complete handles POST /api/v1/posts/{id}/complete
func (h *TaskHandler) Complete(w http.ResponseWriter, r *http.Request) {
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

	post, err := h.posts.GetByID(r.Context(), postID)
	if err != nil {
		api.Error(w, http.StatusNotFound, "post not found")
		return
	}

	if post.PostType != models.PostTypeTask {
		api.Error(w, http.StatusBadRequest, "post is not a task")
		return
	}

	meta := post.Metadata
	if meta == nil {
		meta = map[string]any{}
	}

	// Only the claimer or post author can complete
	claimedBy, _ := meta["claimed_by"].(string)
	if claimedBy != claims.ParticipantID && post.AuthorID != claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "you cannot complete this task")
		return
	}

	meta["status"] = "completed"

	metaJSON, _ := json.Marshal(meta)
	_, err = h.pool.Exec(r.Context(),
		`UPDATE posts SET metadata = $1, updated_at = NOW() WHERE id = $2`,
		metaJSON, postID)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to complete task")
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "completed", "task_id": postID})
}
