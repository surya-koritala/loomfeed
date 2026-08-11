package handlers

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/surya-koritala/loomfeed/internal/api"
	"github.com/surya-koritala/loomfeed/internal/api/middleware"
	"github.com/surya-koritala/loomfeed/internal/config"
	"github.com/surya-koritala/loomfeed/internal/events"
	"github.com/surya-koritala/loomfeed/internal/repository"
	"github.com/surya-koritala/loomfeed/internal/scorecard"
	"github.com/surya-koritala/loomfeed/internal/validate"
)

// EditHandler handles edit, delete, supersede, retract, and revision endpoints.
type EditHandler struct {
	posts      *repository.PostRepo
	comments   *repository.CommentRepo
	revisions  *repository.RevisionRepo
	moderation *repository.ModerationRepo
	// IndexNowPinger interface lives in post.go (same package); wired
	// via WithIndexNow so edits can re-ping search engines.
	indexNow IndexNowPinger
	cfg      *config.Config
	hub      *events.Hub
}

// NewEditHandler creates a new EditHandler.
func NewEditHandler(posts *repository.PostRepo, comments *repository.CommentRepo, revisions *repository.RevisionRepo, cfg *config.Config) *EditHandler {
	return &EditHandler{
		posts:     posts,
		comments:  comments,
		revisions: revisions,
		cfg:       cfg,
	}
}

// WithModeration sets the moderation repo for moderator-level delete authorization.
func (h *EditHandler) WithModeration(moderation *repository.ModerationRepo) {
	h.moderation = moderation
}

// WithIndexNow wires the IndexNow pinger so edits re-submit URLs to
// search engines.
func (h *EditHandler) WithIndexNow(p IndexNowPinger) {
	h.indexNow = p
}

// WithScorecardTrigger wires scorecard recomputation after a retraction.
func (h *EditHandler) WithScorecardTrigger(hub *events.Hub) {
	h.hub = hub
}

// editPostRequest is the request body for PUT /api/v1/posts/{id}.
type editPostRequest struct {
	Title    string         `json:"title"`
	Body     string         `json:"body"`
	PostType string         `json:"post_type,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Tags     []string       `json:"tags,omitempty"`
}

// EditPost handles PUT /api/v1/posts/{id}.
func (h *EditHandler) EditPost(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	// Fetch current post to verify ownership and capture old content for revision.
	post, err := h.posts.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "post not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to fetch post")
		return
	}

	// Check that post is not already deleted.
	// GetByID includes deleted posts from the DB; we handle soft-deleted posts by checking via metadata.
	// Since the model doesn't carry deleted_at, we query raw and trust the Update query to fail gracefully.
	// Ownership check.
	if post.AuthorID != claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "not the post author")
		return
	}

	var req editPostRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validation uses the internal/validate builder so length / shape
	// checks are uniform across handlers. New endpoints should follow
	// the same pattern instead of hand-rolling `if len(...) >` ladders.
	if err := validate.New().
		NotEmpty("title", req.Title).
		StringLen("title", req.Title, 0, 300).
		NotEmpty("body", req.Body).
		StringLen("body", req.Body, 0, 50000).
		Err(); err != nil {
		api.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	postType := req.PostType
	if postType == "" {
		postType = string(post.PostType)
	}

	// Save the current content as a revision before updating.
	if err := h.revisions.Create(r.Context(), id, "post", post.Title, post.Body, post.Metadata); err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to save revision", err)
		return
	}

	metadata := req.Metadata
	if metadata == nil {
		metadata = post.Metadata
	}
	tags := req.Tags
	if tags == nil {
		tags = post.Tags
	}

	if err := h.posts.Update(r.Context(), id, req.Title, req.Body, postType, metadata, tags); err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to update post", err)
		return
	}

	// Re-fetch previews since the body may now reference different URLs.
	if req.Body != "" {
		go PrefetchBodyLinkPreviews(h.posts, id, req.Body)
	}

	// Return the updated post.
	updated, err := h.posts.GetByID(r.Context(), id)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to fetch updated post")
		return
	}

	// Ping IndexNow with the canonical URL so Bing/Yandex re-crawl
	// the edited post. Title may have changed, which changes the
	// slug — submit the new canonical shape.
	if h.indexNow != nil {
		h.indexNow.Fire([]string{indexNowURL(h.cfg, updated.ID, updated.Title)})
	}

	api.JSON(w, http.StatusOK, updated)
}

// DeletePost handles DELETE /api/v1/posts/{id}.
func (h *EditHandler) DeletePost(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	post, err := h.posts.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "post not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to fetch post")
		return
	}

	if post.AuthorID != claims.ParticipantID {
		// Also allow community moderators to delete posts. Skip the
		// moderator lookup when the post has no community at all —
		// otherwise we'd issue a wasteful query and rely on
		// IsModerator's behavior with an empty community_id, which
		// is brittle to changes in that downstream impl.
		if h.moderation == nil || post.CommunityID == "" {
			api.Error(w, http.StatusForbidden, "not the post author")
			return
		}
		isMod, _ := h.moderation.IsModerator(r.Context(), post.CommunityID, claims.ParticipantID)
		if !isMod {
			api.Error(w, http.StatusForbidden, "not the post author or community moderator")
			return
		}
	}

	if err := h.posts.SoftDelete(r.Context(), id); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to delete post")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// editCommentRequest is the request body for PUT /api/v1/comments/{id}.
type editCommentRequest struct {
	Body string `json:"body"`
}

// EditComment handles PUT /api/v1/comments/{id}.
func (h *EditHandler) EditComment(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	comment, err := h.comments.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "comment not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to fetch comment")
		return
	}

	if comment.AuthorID != claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "not the comment author")
		return
	}

	var req editCommentRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := validate.New().
		NotEmpty("body", req.Body).
		StringLen("body", req.Body, 0, 10000).
		Err(); err != nil {
		api.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	// Save old content as a revision.
	if err := h.revisions.Create(r.Context(), id, "comment", "", comment.Body, nil); err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to save revision", err)
		return
	}

	if err := h.comments.Update(r.Context(), id, req.Body); err != nil {
		api.ErrorWithDetail(w, http.StatusInternalServerError, "failed to update comment", err)
		return
	}

	updated, err := h.comments.GetByID(r.Context(), id)
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to fetch updated comment")
		return
	}

	api.JSON(w, http.StatusOK, updated)
}

// DeleteComment handles DELETE /api/v1/comments/{id}.
func (h *EditHandler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	comment, err := h.comments.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "comment not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to fetch comment")
		return
	}

	if comment.AuthorID != claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "not the comment author")
		return
	}

	if err := h.comments.SoftDelete(r.Context(), id); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to delete comment")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// supersedeRequest is the request body for POST /api/v1/posts/{id}/supersede.
type supersedeRequest struct {
	NewPostID string `json:"new_post_id"`
}

// SupersedePost handles POST /api/v1/posts/{id}/supersede.
func (h *EditHandler) SupersedePost(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	oldPost, err := h.posts.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "post not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to fetch post")
		return
	}

	if oldPost.AuthorID != claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "not the post author")
		return
	}

	// Check that the post is not already superseded (double supersede returns 409).
	if oldPost.SupersededBy != nil {
		api.Error(w, http.StatusConflict, "post is already superseded")
		return
	}

	var req supersedeRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.NewPostID == "" {
		api.Error(w, http.StatusBadRequest, "new_post_id is required")
		return
	}

	// Verify the new post exists and belongs to the same author.
	newPost, err := h.posts.GetByID(r.Context(), req.NewPostID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "new post not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to fetch new post")
		return
	}

	if newPost.AuthorID != claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "new post is not authored by you")
		return
	}

	if err := h.posts.Supersede(r.Context(), id, req.NewPostID); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to supersede post")
		return
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "superseded", "new_post_id": req.NewPostID})
}

// retractRequest is the request body for POST /api/v1/posts/{id}/retract.
type retractRequest struct {
	Notice string `json:"notice"`
}

// RetractPost handles POST /api/v1/posts/{id}/retract.
func (h *EditHandler) RetractPost(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		api.Error(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	post, err := h.posts.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			api.Error(w, http.StatusNotFound, "post not found")
			return
		}
		api.Error(w, http.StatusInternalServerError, "failed to fetch post")
		return
	}

	if post.AuthorID != claims.ParticipantID {
		api.Error(w, http.StatusForbidden, "not the post author")
		return
	}

	var req retractRequest
	if err := api.Decode(r, &req); err != nil {
		api.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Notice == "" {
		api.Error(w, http.StatusBadRequest, "notice is required")
		return
	}

	if err := h.posts.Retract(r.Context(), id, req.Notice); err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to retract post")
		return
	}
	if h.hub != nil {
		scorecard.TriggerCompute(h.hub, post.AuthorID)
	}

	api.JSON(w, http.StatusOK, map[string]string{"status": "retracted"})
}

// GetRevisions handles GET /api/v1/posts/{id}/revisions.
func (h *EditHandler) GetRevisions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	revisions, err := h.revisions.ListByContent(r.Context(), id, "post")
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to fetch revisions")
		return
	}

	if revisions == nil {
		revisions = []repository.Revision{}
	}

	api.JSON(w, http.StatusOK, revisions)
}

// GetCommentRevisions handles GET /api/v1/comments/{id}/revisions.
func (h *EditHandler) GetCommentRevisions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		api.Error(w, http.StatusBadRequest, "id is required")
		return
	}

	revisions, err := h.revisions.ListByContent(r.Context(), id, "comment")
	if err != nil {
		api.Error(w, http.StatusInternalServerError, "failed to fetch revisions")
		return
	}

	if revisions == nil {
		revisions = []repository.Revision{}
	}

	api.JSON(w, http.StatusOK, revisions)
}
