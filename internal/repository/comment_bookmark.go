package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CommentBookmarkRepo handles database operations for comment bookmarks.
type CommentBookmarkRepo struct {
	pool *pgxpool.Pool
}

// NewCommentBookmarkRepo creates a new CommentBookmarkRepo.
func NewCommentBookmarkRepo(pool *pgxpool.Pool) *CommentBookmarkRepo {
	return &CommentBookmarkRepo{pool: pool}
}

// Toggle adds or removes a comment bookmark. Returns true if bookmarked, false if removed.
func (r *CommentBookmarkRepo) Toggle(ctx context.Context, participantID, commentID string) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM comment_bookmarks WHERE participant_id = $1 AND comment_id = $2`,
		participantID, commentID)
	if err != nil {
		return false, fmt.Errorf("delete comment bookmark: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return false, nil // removed
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO comment_bookmarks (participant_id, comment_id) VALUES ($1, $2)`,
		participantID, commentID)
	if err != nil {
		return false, fmt.Errorf("insert comment bookmark: %w", err)
	}
	return true, nil // added
}

// ListByParticipant returns comment IDs bookmarked by a participant.
func (r *CommentBookmarkRepo) ListByParticipant(ctx context.Context, participantID string, limit, offset int) ([]string, int, error) {
	var total int
	_ = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM comment_bookmarks WHERE participant_id = $1`,
		participantID).Scan(&total)

	rows, err := r.pool.Query(ctx,
		`SELECT comment_id FROM comment_bookmarks WHERE participant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		participantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	return ids, total, rows.Err()
}

// BookmarkedComment is the rendering-ready shape returned by
// ListByParticipantWithDetails — enough for the bookmarks page to
// show a comment card without N+1 follow-up queries.
type BookmarkedComment struct {
	ID          string  `json:"id"`
	PostID      string  `json:"post_id"`
	PostTitle   string  `json:"post_title,omitempty"`
	PostSlug    string  `json:"post_slug,omitempty"`
	Body        string  `json:"body"`
	VoteScore   int     `json:"vote_score"`
	CreatedAt   string  `json:"created_at"`
	BookmarkAt  string  `json:"bookmarked_at"`
	AuthorID    string  `json:"author_id,omitempty"`
	AuthorName  string  `json:"author_display_name,omitempty"`
	AuthorType  string  `json:"author_type,omitempty"`
	AuthorAvatar *string `json:"author_avatar_url,omitempty"`
}

// ListByParticipantWithDetails returns saved-comment cards in one
// query — body + author + parent-post title — so the bookmarks page
// renders without an N+1 fetch loop. Skips soft-deleted comments and
// any whose parent post has been removed (those entries linger in
// comment_bookmarks but aren't worth showing).
func (r *CommentBookmarkRepo) ListByParticipantWithDetails(ctx context.Context, participantID string, limit, offset int) ([]BookmarkedComment, int, error) {
	var total int
	_ = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM comment_bookmarks cb
		 JOIN comments c ON c.id = cb.comment_id AND c.deleted_at IS NULL
		 JOIN posts p ON p.id = c.post_id AND p.deleted_at IS NULL
		 WHERE cb.participant_id = $1`,
		participantID).Scan(&total)

	rows, err := r.pool.Query(ctx, `
		SELECT
		  c.id,
		  c.post_id,
		  COALESCE(p.title, ''),
		  COALESCE(p.slug, ''),
		  COALESCE(c.body, ''),
		  COALESCE(c.vote_score, 0),
		  c.created_at,
		  cb.created_at,
		  COALESCE(a.id::text, ''),
		  COALESCE(a.display_name, ''),
		  COALESCE(a.type::text, ''),
		  a.avatar_url
		FROM comment_bookmarks cb
		JOIN comments c ON c.id = cb.comment_id AND c.deleted_at IS NULL
		JOIN posts p ON p.id = c.post_id AND p.deleted_at IS NULL
		LEFT JOIN participants a ON a.id = c.author_id
		WHERE cb.participant_id = $1
		ORDER BY cb.created_at DESC
		LIMIT $2 OFFSET $3`,
		participantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list bookmarked comments with details: %w", err)
	}
	defer rows.Close()

	out := make([]BookmarkedComment, 0)
	for rows.Next() {
		var b BookmarkedComment
		var createdAt, bookmarkAt any
		if err := rows.Scan(
			&b.ID, &b.PostID, &b.PostTitle, &b.PostSlug,
			&b.Body, &b.VoteScore, &createdAt, &bookmarkAt,
			&b.AuthorID, &b.AuthorName, &b.AuthorType, &b.AuthorAvatar,
		); err != nil {
			return nil, 0, fmt.Errorf("scan bookmarked comment: %w", err)
		}
		if t, ok := createdAt.(interface{ Format(string) string }); ok {
			b.CreatedAt = t.Format("2006-01-02T15:04:05Z07:00")
		}
		if t, ok := bookmarkAt.(interface{ Format(string) string }); ok {
			b.BookmarkAt = t.Format("2006-01-02T15:04:05Z07:00")
		}
		out = append(out, b)
	}
	return out, total, rows.Err()
}

// GetUserBookmarksForComments returns a set of comment IDs that the participant has bookmarked.
func (r *CommentBookmarkRepo) GetUserBookmarksForComments(ctx context.Context, participantID string, commentIDs []string) (map[string]bool, error) {
	if len(commentIDs) == 0 {
		return nil, nil
	}

	args := []any{participantID}
	placeholders := make([]string, len(commentIDs))
	for i, id := range commentIDs {
		args = append(args, id)
		placeholders[i] = fmt.Sprintf("$%d", i+2)
	}

	query := `SELECT comment_id FROM comment_bookmarks WHERE participant_id = $1 AND comment_id IN (` + strings.Join(placeholders, ",") + `)`
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get user comment bookmarks: %w", err)
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var commentID string
		if err := rows.Scan(&commentID); err != nil {
			return nil, fmt.Errorf("scan comment bookmark: %w", err)
		}
		result[commentID] = true
	}
	return result, rows.Err()
}

// IsBookmarked returns whether a participant has bookmarked a comment.
func (r *CommentBookmarkRepo) IsBookmarked(ctx context.Context, participantID, commentID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM comment_bookmarks WHERE participant_id = $1 AND comment_id = $2)`,
		participantID, commentID).Scan(&exists)
	return exists, err
}
