package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/database"
	"github.com/surya-koritala/loomfeed/internal/models"
)

// CommentRepo handles database operations for comments.
type CommentRepo struct {
	pool *pgxpool.Pool
}

// NewCommentRepo creates a new CommentRepo.
func NewCommentRepo(pool *pgxpool.Pool) *CommentRepo {
	return &CommentRepo{pool: pool}
}

// IsDuplicate checks two things:
// 1. Same author posted the same comment on the same post (any time)
// 2. Same comment body posted by ANY author on 3+ different posts in the last 24h (cross-post spam)
// Returns true if either condition is met.
func (r *CommentRepo) IsDuplicate(ctx context.Context, postID, authorID, body string) (bool, error) {
	var exists bool
	// Check 1: exact duplicate on same post by same author
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM comments
			WHERE post_id = $1 AND author_id = $2 AND body = $3
			  AND deleted_at IS NULL
		)`, postID, authorID, body).Scan(&exists)
	if err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}
	// Check 2: cross-post spam — same body on 3+ posts in the last 24 hours
	var crossPostCount int
	err = r.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT post_id) FROM comments
		WHERE author_id = $1 AND body = $2
		  AND deleted_at IS NULL AND created_at > NOW() - INTERVAL '24 hours'`,
		authorID, body).Scan(&crossPostCount)
	if err != nil {
		return false, err
	}
	return crossPostCount >= 3, nil
}

// Pool returns the underlying database pool (used by handlers for ad-hoc queries).
func (r *CommentRepo) Pool() *pgxpool.Pool {
	return r.pool
}

// Create inserts a new comment.
// If parent_comment_id is set, depth = parent_depth + 1; otherwise depth = 0.
// Also atomically increments the post's comment_count and the author's
// comment_count on the participants table using CTEs to reduce round-trips.
func (r *CommentRepo) Create(ctx context.Context, c *models.Comment) (*models.Comment, error) {
	var result *models.Comment
	err := database.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		created, err := createComment(ctx, tx, c)
		result = created
		return err
	})
	return result, err
}

// CreateWithProvenance creates a comment, its provenance row, and the durable
// comments.provenance_id link in one transaction. The comment and both counter
// increments roll back if provenance insertion or attachment fails.
func (r *CommentRepo) CreateWithProvenance(ctx context.Context, c *models.Comment, provenance *models.Provenance) (*models.Comment, error) {
	var result *models.Comment
	err := database.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		created, err := createComment(ctx, tx, c)
		if err != nil {
			return err
		}

		provenance.ContentID = created.ID
		provenance.ContentType = models.TargetComment
		provenance.AuthorID = created.AuthorID
		createdProvenance, err := createProvenance(ctx, tx, provenance)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx,
			`UPDATE comments SET provenance_id = $1 WHERE id = $2`,
			createdProvenance.ID, created.ID,
		); err != nil {
			return fmt.Errorf("attach comment provenance: %w", err)
		}
		created.ProvenanceID = &createdProvenance.ID
		result = created
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func createComment(ctx context.Context, db database.DBTX, c *models.Comment) (*models.Comment, error) {
	depth := 0
	if c.ParentCommentID != nil && *c.ParentCommentID != "" {
		// Parent depth lookup still needed — cannot be combined into the CTE
		// because we need the depth value for the INSERT.
		var parentDepth int
		err := db.QueryRow(ctx, `SELECT depth FROM comments WHERE id = $1 AND deleted_at IS NULL`, *c.ParentCommentID).Scan(&parentDepth)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("parent comment not found: %w", pgx.ErrNoRows)
			}
			return nil, fmt.Errorf("query parent depth: %w", err)
		}
		depth = parentDepth + 1
	}

	// Single-query CTE: insert comment + bump post comment_count +
	// bump participant comment_count — all in one round-trip.
	var result models.Comment
	// Default thread is "main" — talk page opt-in via request.
	threadType := c.ThreadType
	if threadType != "talk" {
		threadType = "main"
	}

	err := db.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO comments
			  (post_id, parent_comment_id, author_id, author_type, body,
			   provenance_id, confidence_score, depth, is_answer, thread_type)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING
			  id, post_id, parent_comment_id, author_id, author_type,
			  body, provenance_id, confidence_score,
			  vote_score, depth, is_answer, thread_type, created_at, updated_at
		), bump_post AS (
			UPDATE posts SET comment_count = comment_count + 1 WHERE id = $1 AND $10 = 'main'
		), bump_participant AS (
			UPDATE participants SET comment_count = comment_count + 1 WHERE id = $3 AND $10 = 'main'
		)
		SELECT * FROM inserted`,
		c.PostID,
		c.ParentCommentID,
		c.AuthorID,
		c.AuthorType,
		c.Body,
		c.ProvenanceID,
		c.ConfidenceScore,
		depth,
		c.IsAnswer,
		threadType,
	).Scan(
		&result.ID, &result.PostID, &result.ParentCommentID,
		&result.AuthorID, &result.AuthorType,
		&result.Body, &result.ProvenanceID, &result.ConfidenceScore,
		&result.VoteScore, &result.Depth, &result.IsAnswer, &result.ThreadType, &result.CreatedAt, &result.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert comment: %w", err)
	}
	var public bool
	if err := db.QueryRow(ctx, `
		SELECT p.deleted_at IS NULL AND NOT p.quarantined
		FROM posts p WHERE p.id = $1
		FOR SHARE`, result.PostID).Scan(&public); err != nil {
		return nil, fmt.Errorf("check comment webhook visibility: %w", err)
	}
	if public {
		if _, err := enqueueWebhookEvent(ctx, db, "comment.created", map[string]any{
			"comment_id": result.ID, "post_id": result.PostID,
			"author_id": result.AuthorID, "body_excerpt": webhookExcerpt(result.Body, 200),
		}); err != nil {
			return nil, fmt.Errorf("enqueue comment.created: %w", err)
		}
	}

	return &result, nil
}

// CreateLoomReplyParams is the surface the Loom worker uses to post
// its reply as a regular comment authored by the Loom participant.
type CreateLoomReplyParams struct {
	PostID          string
	ParentCommentID *string
	Body            string
	LoomSummonID    string
	LoomIntent      string
}

// CreateLoomReply inserts a comment authored by the Loom participant
// and tags it with the originating summon. Distinct from Create on two
// counts: it does NOT bump posts.comment_count and does NOT bump the
// Loom participant's comment_count. Loom replies are platform output,
// not user engagement, so the counters that drive sorting and profile
// stats stay clean.
func (r *CommentRepo) CreateLoomReply(ctx context.Context, p CreateLoomReplyParams) (*models.Comment, error) {
	depth := 0
	if p.ParentCommentID != nil && *p.ParentCommentID != "" {
		var parentDepth int
		err := r.pool.QueryRow(ctx,
			`SELECT depth FROM comments WHERE id = $1 AND deleted_at IS NULL`,
			*p.ParentCommentID,
		).Scan(&parentDepth)
		if err != nil {
			return nil, fmt.Errorf("query parent depth for loom reply: %w", err)
		}
		depth = parentDepth + 1
	}

	var c models.Comment
	err := r.pool.QueryRow(ctx, `
		INSERT INTO comments
			(post_id, parent_comment_id, author_id, author_type, body,
			 depth, thread_type, loom_summon_id, loom_intent)
		VALUES ($1, $2, $3, 'loom', $4, $5, 'main', $6, $7)
		RETURNING
			id, post_id, parent_comment_id, author_id, author_type,
			body, vote_score, depth, is_answer, thread_type,
			created_at, updated_at, loom_summon_id, loom_intent`,
		p.PostID, p.ParentCommentID, models.LoomParticipantID,
		p.Body, depth, p.LoomSummonID, p.LoomIntent,
	).Scan(
		&c.ID, &c.PostID, &c.ParentCommentID,
		&c.AuthorID, &c.AuthorType,
		&c.Body, &c.VoteScore, &c.Depth, &c.IsAnswer, &c.ThreadType,
		&c.CreatedAt, &c.UpdatedAt, &c.LoomSummonID, &c.LoomIntent,
	)
	if err != nil {
		return nil, fmt.Errorf("insert loom reply: %w", err)
	}
	return &c, nil
}

// GetByID returns a single comment by its ID.
func (r *CommentRepo) GetByID(ctx context.Context, id string) (*models.Comment, error) {
	var c models.Comment
	err := r.pool.QueryRow(ctx, `
		SELECT id, post_id, parent_comment_id, author_id, author_type,
		       body, provenance_id, confidence_score,
		       vote_score, depth, is_answer, created_at, updated_at
		FROM comments WHERE id = $1`, id).Scan(
		&c.ID, &c.PostID, &c.ParentCommentID, &c.AuthorID, &c.AuthorType,
		&c.Body, &c.ProvenanceID, &c.ConfidenceScore,
		&c.VoteScore, &c.Depth, &c.IsAnswer, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get comment by id: %w", err)
	}
	return &c, nil
}

// GetByIDWithAuthor returns a single comment with joined author data.
func (r *CommentRepo) GetByIDWithAuthor(ctx context.Context, id string) (*models.CommentWithAuthor, error) {
	var cwa models.CommentWithAuthor
	var provenanceSources []string
	var provenanceConfidence *float64
	var provenanceMethod *string
	err := r.pool.QueryRow(ctx, `
		SELECT
			c.id, c.post_id, c.parent_comment_id, c.author_id, c.author_type,
			c.body, c.provenance_id, c.confidence_score,
			c.vote_score, c.depth, c.is_answer, c.created_at, c.updated_at,
			c.loom_summon_id, c.loom_intent,
			p.id, p.type, p.display_name,
			COALESCE(p.avatar_url, '') AS avatar_url,
			COALESCE(p.bio, '') AS bio,
			p.trust_score, p.reputation_score, p.is_verified, p.created_at, p.updated_at,
			COALESCE(prov.sources, '{}') AS provenance_sources,
			prov.confidence_score, prov.generation_method::text
		FROM comments c
		JOIN participants p ON p.id = c.author_id
		LEFT JOIN provenances prov ON prov.id = c.provenance_id
		WHERE c.id = $1`, id).Scan(
		&cwa.ID, &cwa.PostID, &cwa.ParentCommentID, &cwa.AuthorID, &cwa.AuthorType,
		&cwa.Body, &cwa.ProvenanceID, &cwa.ConfidenceScore,
		&cwa.VoteScore, &cwa.Depth, &cwa.IsAnswer, &cwa.CreatedAt, &cwa.UpdatedAt,
		&cwa.LoomSummonID, &cwa.LoomIntent,
		&cwa.Author.ID, &cwa.Author.Type, &cwa.Author.DisplayName,
		&cwa.Author.AvatarURL, &cwa.Author.Bio,
		&cwa.Author.TrustScore, &cwa.Author.ReputationScore, &cwa.Author.IsVerified,
		&cwa.Author.CreatedAt, &cwa.Author.UpdatedAt,
		&provenanceSources, &provenanceConfidence, &provenanceMethod,
	)
	if err != nil {
		return nil, fmt.Errorf("get comment with author: %w", err)
	}
	populateCommentProvenance(&cwa, provenanceSources, provenanceConfidence, provenanceMethod)
	return &cwa, nil
}

func populateCommentProvenance(c *models.CommentWithAuthor, sources []string, confidence *float64, method *string) {
	if c.ProvenanceID == nil {
		return
	}
	if sources == nil {
		sources = []string{}
	}
	provenance := &models.Provenance{
		ID:          *c.ProvenanceID,
		ContentID:   c.ID,
		ContentType: models.TargetComment,
		AuthorID:    c.AuthorID,
		Sources:     sources,
	}
	if confidence != nil {
		provenance.ConfidenceScore = *confidence
	}
	if method != nil {
		provenance.GenerationMethod = models.GenerationMethod(*method)
	}
	c.Provenance = provenance
}

// Update edits an existing comment's body. Only updates non-deleted comments.
func (r *CommentRepo) Update(ctx context.Context, id, body string) error {
	_, err := r.pool.Exec(ctx, `UPDATE comments SET body = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`, body, id)
	return err
}

// SoftDelete marks a comment as deleted by setting deleted_at.
func (r *CommentRepo) SoftDelete(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE comments SET deleted_at = NOW() WHERE id = $1`, id)
	return err
}

// commentSortClause returns the ORDER BY expression for the given sort mode.
// The depth column is always the primary sort key so threading is preserved.
func commentSortClause(sort string) string {
	switch sort {
	case "new":
		return "c.depth ASC, c.created_at DESC, c.id DESC"
	case "old":
		return "c.depth ASC, c.created_at ASC, c.id DESC"
	case "controversial":
		return "c.depth ASC, (c.upvote_count + c.downvote_count) DESC, ABS(c.upvote_count - c.downvote_count) ASC, c.id DESC"
	default: // "best" — Wilson score confidence interval
		return `c.depth ASC, ` + commentBestScore("c") + ` DESC, c.id DESC`
	}
}

func commentBestScore(alias string) string {
	return fmt.Sprintf(`(CASE WHEN (%[1]s.upvote_count + %[1]s.downvote_count) = 0 THEN 0 ELSE ((%[1]s.upvote_count + 1.9208) / (%[1]s.upvote_count + %[1]s.downvote_count) - 1.96 * SQRT((%[1]s.upvote_count * %[1]s.downvote_count::float) / (%[1]s.upvote_count + %[1]s.downvote_count) + 0.9604) / (%[1]s.upvote_count + %[1]s.downvote_count)) / (1 + 3.8416 / (%[1]s.upvote_count + %[1]s.downvote_count)) END)`, alias)
}

// commentCursorClause mirrors commentSortClause, including its mixed ASC/DESC
// directions. cursor_anchor is a single row joined only for cursor requests.
func commentCursorClause(sort string) string {
	depthAfter := `c.depth > cursor_anchor.depth`
	sameDepth := `c.depth = cursor_anchor.depth`
	switch sort {
	case "new":
		return `(` + depthAfter + ` OR (` + sameDepth + ` AND (c.created_at, c.id) < (cursor_anchor.created_at, cursor_anchor.id)))`
	case "old":
		return `(` + depthAfter + ` OR (` + sameDepth + ` AND (c.created_at > cursor_anchor.created_at OR (c.created_at = cursor_anchor.created_at AND c.id < cursor_anchor.id))))`
	case "controversial":
		return `(` + depthAfter + ` OR (` + sameDepth + ` AND (
			(c.upvote_count + c.downvote_count) < (cursor_anchor.upvote_count + cursor_anchor.downvote_count)
			OR ((c.upvote_count + c.downvote_count) = (cursor_anchor.upvote_count + cursor_anchor.downvote_count) AND (
				ABS(c.upvote_count - c.downvote_count) > ABS(cursor_anchor.upvote_count - cursor_anchor.downvote_count)
				OR (ABS(c.upvote_count - c.downvote_count) = ABS(cursor_anchor.upvote_count - cursor_anchor.downvote_count) AND c.id < cursor_anchor.id)
			))
		)))`
	default:
		currentScore := commentBestScore("c")
		anchorScore := commentBestScore("cursor_anchor")
		return fmt.Sprintf(`(%s OR (%s AND (%s < %s OR (%s = %s AND c.id < cursor_anchor.id))))`,
			depthAfter, sameDepth, currentScore, anchorScore, currentScore, anchorScore)
	}
}

// ListByPost returns comments for a post joined with author participant data.
// sort controls ordering: "best" (default), "new", "old", "controversial".
// mode filters by answer status: "answers" (is_answer=true), "comments" (is_answer=false), or "" (all).
// threadType: "main" (default) or "talk" — the Wikipedia-style meta-thread.
func (r *CommentRepo) ListByPost(ctx context.Context, postID string, sort string, limit, offset int, mode string, threadType string, cursor ...string) ([]models.CommentWithAuthor, error) {
	orderBy := commentSortClause(sort)

	modeFilter := ""
	switch mode {
	case "answers":
		modeFilter = " AND c.is_answer = TRUE"
	case "comments":
		modeFilter = " AND c.is_answer = FALSE"
	}

	// Thread filter — defaults to the main conversation.
	if threadType != "talk" {
		threadType = "main"
	}
	modeFilter += " AND c.thread_type = '" + threadType + "'"

	args := []any{postID}
	anchorJoin := ""
	cursorFilter := ""
	useCursor := len(cursor) > 0 && cursor[0] != ""
	if useCursor {
		args = append(args, cursor[0])
		cursorParam := fmt.Sprintf("$%d", len(args))
		anchorJoin = `
		CROSS JOIN (SELECT * FROM comments WHERE id = ` + cursorParam + ` AND post_id = $1) cursor_anchor`
		cursorFilter = " AND " + commentCursorClause(sort)
	}
	args = append(args, limit)
	limitParam := fmt.Sprintf("$%d", len(args))
	offsetClause := ""
	if !useCursor {
		args = append(args, offset)
		offsetClause = fmt.Sprintf(" OFFSET $%d", len(args))
	}

	rows, err := r.pool.Query(ctx, `
		SELECT
			c.id, c.post_id, c.parent_comment_id, c.author_id, c.author_type,
			c.body, c.provenance_id, c.confidence_score,
			c.vote_score, c.depth, c.is_answer, c.created_at, c.updated_at,
			c.loom_summon_id, c.loom_intent,
			p.id, p.type, p.display_name,
			COALESCE(p.avatar_url, '') AS avatar_url,
			COALESCE(p.bio, '') AS bio,
			p.trust_score, p.reputation_score, p.is_verified, p.created_at, p.updated_at,
			COALESCE(a.model_provider, '') AS model_provider,
			COALESCE(a.model_name, '') AS model_name,
			COALESCE(prov.sources, '{}') AS provenance_sources,
			prov.confidence_score, prov.generation_method::text
		FROM comments c
		JOIN participants p ON p.id = c.author_id
		LEFT JOIN agent_identities a ON a.participant_id = c.author_id
		LEFT JOIN provenances prov ON prov.id = c.provenance_id
		`+anchorJoin+`
		WHERE c.post_id = $1`+modeFilter+cursorFilter+`
		ORDER BY `+orderBy+`
		LIMIT `+limitParam+offsetClause,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("list comments by post: %w", err)
	}
	defer rows.Close()

	var comments []models.CommentWithAuthor
	for rows.Next() {
		var cwa models.CommentWithAuthor
		var modelProvider, modelName string
		var provenanceSources []string
		var provenanceConfidence *float64
		var provenanceMethod *string
		if err := rows.Scan(
			&cwa.ID, &cwa.PostID, &cwa.ParentCommentID, &cwa.AuthorID, &cwa.AuthorType,
			&cwa.Body, &cwa.ProvenanceID, &cwa.ConfidenceScore,
			&cwa.VoteScore, &cwa.Depth, &cwa.IsAnswer, &cwa.CreatedAt, &cwa.UpdatedAt,
			&cwa.LoomSummonID, &cwa.LoomIntent,
			&cwa.Author.ID, &cwa.Author.Type, &cwa.Author.DisplayName,
			&cwa.Author.AvatarURL, &cwa.Author.Bio,
			&cwa.Author.TrustScore, &cwa.Author.ReputationScore, &cwa.Author.IsVerified,
			&cwa.Author.CreatedAt, &cwa.Author.UpdatedAt,
			&modelProvider, &modelName,
			&provenanceSources, &provenanceConfidence, &provenanceMethod,
		); err != nil {
			return nil, fmt.Errorf("scanning comment row: %w", err)
		}
		cwa.Author.ModelProvider = modelProvider
		cwa.Author.ModelName = modelName
		populateCommentProvenance(&cwa, provenanceSources, provenanceConfidence, provenanceMethod)
		comments = append(comments, cwa)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating comment rows: %w", err)
	}

	return comments, nil
}

// GetAncestorChain walks the parent_comment_id chain upward from
// `id` until it hits the root (parent_comment_id IS NULL) and
// returns the chain in root-most-first order. Used by the comment
// permalink page to render the "Replying to a thread by …"
// breadcrumb.
//
// Each row returns just enough to render a breadcrumb chip
// (id, display_name) — full author / body fields are NOT pulled,
// so this stays cheap even on deeply-nested chains.
func (r *CommentRepo) GetAncestorChain(ctx context.Context, id string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `
		WITH RECURSIVE chain AS (
			SELECT c.id, c.parent_comment_id, c.author_id, p.display_name, 1 AS depth_up
			FROM comments c
			JOIN participants p ON p.id = c.author_id
			WHERE c.id = $1 AND c.deleted_at IS NULL
			UNION ALL
			SELECT c.id, c.parent_comment_id, c.author_id, p.display_name, chain.depth_up + 1
			FROM comments c
			JOIN participants p ON p.id = c.author_id
			JOIN chain ON chain.parent_comment_id = c.id
			WHERE c.deleted_at IS NULL
		)
		SELECT id, COALESCE(parent_comment_id::text, ''), author_id, display_name
		FROM chain
		WHERE id <> $1
		ORDER BY depth_up DESC`, id)
	if err != nil {
		return nil, fmt.Errorf("ancestor chain query: %w", err)
	}
	defer rows.Close()

	chain := []map[string]any{}
	for rows.Next() {
		var cid, parentID, authorID, displayName string
		if err := rows.Scan(&cid, &parentID, &authorID, &displayName); err != nil {
			return nil, fmt.Errorf("scan ancestor row: %w", err)
		}
		entry := map[string]any{
			"id":           cid,
			"author_id":    authorID,
			"display_name": displayName,
		}
		if parentID != "" {
			entry["parent_comment_id"] = parentID
		}
		chain = append(chain, entry)
	}
	return chain, rows.Err()
}

// ListDescendants returns the descendant subtree rooted at `id`,
// capped at `maxDepth` levels of nesting and `limit` rows total.
// Returns CommentWithAuthor flat; the frontend stitches the tree by
// parent_comment_id.
//
// Sort: depth ASC, created_at ASC — closer-to-root comments come
// first so a hard limit still yields a coherent breadth-first slice
// instead of one deep branch dominating the result set.
//
// To preserve "load everything" behaviour for legacy callers, pass
// maxDepth=0 and limit=0 (both treated as unbounded).
func (r *CommentRepo) ListDescendants(ctx context.Context, id string, maxDepth, limit int) ([]models.CommentWithAuthor, error) {
	// Build the CTE depth filter and outer LIMIT inline rather than
	// wiring them through positional args — pgx prepared-statement
	// layer doesn't love optional clauses, and these are tiny ints.
	// The depth filter is appended to the recursive branch's existing
	// WHERE clause, so use AND (not WHERE). Compares against rel_depth,
	// which is the synthesized CTE column tracking distance from the
	// permalink root — not comments.depth, which is the post-level
	// depth fixed at insert time.
	depthFilter := ""
	if maxDepth > 0 {
		depthFilter = fmt.Sprintf("AND d.rel_depth + 1 <= %d", maxDepth)
	}
	limitClause := ""
	if limit > 0 {
		limitClause = fmt.Sprintf("LIMIT %d", limit)
	}

	q := fmt.Sprintf(`
		WITH RECURSIVE descendants AS (
			SELECT c.*, 1 AS rel_depth
			FROM comments c
			WHERE c.parent_comment_id = $1 AND c.deleted_at IS NULL
			UNION ALL
			SELECT c.*, d.rel_depth + 1
			FROM comments c
			JOIN descendants d ON c.parent_comment_id = d.id
			WHERE c.deleted_at IS NULL %s
		)
		SELECT
			c.id, c.post_id, c.parent_comment_id, c.author_id, c.author_type,
			c.body, c.provenance_id, c.confidence_score,
			c.vote_score, c.depth, c.is_answer, c.created_at, c.updated_at,
			c.loom_summon_id, c.loom_intent,
			p.id, p.type, p.display_name,
			COALESCE(p.avatar_url, '') AS avatar_url,
			COALESCE(p.bio, '') AS bio,
			p.trust_score, p.reputation_score, p.is_verified, p.created_at, p.updated_at,
			COALESCE(prov.sources, '{}') AS provenance_sources,
			prov.confidence_score, prov.generation_method::text
		FROM descendants c
		JOIN participants p ON p.id = c.author_id
		LEFT JOIN provenances prov ON prov.id = c.provenance_id
		ORDER BY c.rel_depth ASC, c.created_at ASC %s`, depthFilter, limitClause)

	rows, err := r.pool.Query(ctx, q, id)
	if err != nil {
		return nil, fmt.Errorf("descendants query: %w", err)
	}
	defer rows.Close()

	out := []models.CommentWithAuthor{}
	for rows.Next() {
		var cwa models.CommentWithAuthor
		var provenanceSources []string
		var provenanceConfidence *float64
		var provenanceMethod *string
		if err := rows.Scan(
			&cwa.ID, &cwa.PostID, &cwa.ParentCommentID, &cwa.AuthorID, &cwa.AuthorType,
			&cwa.Body, &cwa.ProvenanceID, &cwa.ConfidenceScore,
			&cwa.VoteScore, &cwa.Depth, &cwa.IsAnswer, &cwa.CreatedAt, &cwa.UpdatedAt,
			&cwa.LoomSummonID, &cwa.LoomIntent,
			&cwa.Author.ID, &cwa.Author.Type, &cwa.Author.DisplayName,
			&cwa.Author.AvatarURL, &cwa.Author.Bio,
			&cwa.Author.TrustScore, &cwa.Author.ReputationScore, &cwa.Author.IsVerified,
			&cwa.Author.CreatedAt, &cwa.Author.UpdatedAt,
			&provenanceSources, &provenanceConfidence, &provenanceMethod,
		); err != nil {
			return nil, fmt.Errorf("scan descendant row: %w", err)
		}
		populateCommentProvenance(&cwa, provenanceSources, provenanceConfidence, provenanceMethod)
		out = append(out, cwa)
	}
	return out, rows.Err()
}
