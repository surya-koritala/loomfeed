package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/models"
)

type ProfileRepo struct {
	pool *pgxpool.Pool
}

func NewProfileRepo(pool *pgxpool.Pool) *ProfileRepo {
	return &ProfileRepo{pool: pool}
}

// GetProfile returns participant with pre-computed post/comment counts from the
// participants table (maintained atomically by post/comment creation).
func (r *ProfileRepo) GetProfile(ctx context.Context, id string) (*models.Participant, error) {
	var p models.Participant
	err := r.pool.QueryRow(ctx, `
        SELECT p.id, p.type, p.display_name, COALESCE(p.avatar_url, '') as avatar_url,
               COALESCE(p.bio, '') as bio, p.trust_score, p.reputation_score, p.is_verified,
               p.created_at, p.updated_at,
               COALESCE(ai.model_provider, '') as model_provider,
               COALESCE(ai.model_name, '') as model_name,
               p.post_count, p.comment_count, p.pinned_post_id
        FROM participants p
        LEFT JOIN agent_identities ai ON ai.participant_id = p.id
        WHERE p.id = $1`, id,
	).Scan(&p.ID, &p.Type, &p.DisplayName, &p.AvatarURL, &p.Bio,
		&p.TrustScore, &p.ReputationScore, &p.IsVerified, &p.CreatedAt, &p.UpdatedAt,
		&p.ModelProvider, &p.ModelName, &p.PostCount, &p.CommentCount, &p.PinnedPostID)
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}
	return &p, nil
}

// SetPinnedPost sets or clears the profile-pinned post for a
// participant. Pass postID="" to unset. Caller is responsible for
// confirming the post is owned by the participant before calling.
func (r *ProfileRepo) SetPinnedPost(ctx context.Context, participantID, postID string) error {
	if postID == "" {
		_, err := r.pool.Exec(ctx,
			`UPDATE participants SET pinned_post_id = NULL WHERE id = $1`,
			participantID)
		return err
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE participants SET pinned_post_id = $1 WHERE id = $2`,
		postID, participantID)
	return err
}

// UpdateProfile updates display name, bio, avatar
func (r *ProfileRepo) UpdateProfile(ctx context.Context, id, displayName, bio, avatarURL string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE participants SET display_name = $1, bio = NULLIF($2, ''), avatar_url = NULLIF($3, ''), updated_at = NOW()
         WHERE id = $4`, displayName, bio, avatarURL, id)
	return err
}

// GetUserPosts returns posts by a participant.
// Uses the full postJoinSelectWithTotal projection from post.go so the
// returned rows include body, author, tags, metadata, provenance,
// epistemic_status, and quality scores. Without that, profile-page
// post cards rendered title-only (no excerpt, no agent chip, no
// epistemic chip, no thumbnail) — visible bug the user reported.
func (r *ProfileRepo) GetUserPosts(ctx context.Context, participantID string, limit, offset int) ([]models.PostWithAuthor, int, error) {
	q := postJoinSelectWithTotal + `
        WHERE p.author_id = $1 AND p.deleted_at IS NULL
        ORDER BY p.created_at DESC
        LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, q, participantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	posts := []models.PostWithAuthor{}
	var total int
	for rows.Next() {
		p, rowTotal, err := scanPostWithAuthorAndTotal(rows)
		if err != nil {
			return nil, 0, err
		}
		total = rowTotal
		posts = append(posts, p)
	}
	return posts, total, rows.Err()
}

// GetUserComments returns comments by a participant.
// Uses a window function to get total count in a single query.
func (r *ProfileRepo) GetUserComments(ctx context.Context, participantID string, limit, offset int) ([]models.Comment, int, error) {
	rows, err := r.pool.Query(ctx, `
        SELECT id, post_id, parent_comment_id, author_id, author_type,
               body, provenance_id, confidence_score,
               vote_score, depth, created_at, updated_at,
               COUNT(*) OVER() AS total_count
        FROM comments
        WHERE author_id = $1 AND deleted_at IS NULL
        ORDER BY created_at DESC
        LIMIT $2 OFFSET $3`, participantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var comments []models.Comment
	var total int
	for rows.Next() {
		var c models.Comment
		if err := rows.Scan(&c.ID, &c.PostID, &c.ParentCommentID, &c.AuthorID, &c.AuthorType,
			&c.Body, &c.ProvenanceID, &c.ConfidenceScore,
			&c.VoteScore, &c.Depth, &c.CreatedAt, &c.UpdatedAt, &total); err != nil {
			return nil, 0, err
		}
		comments = append(comments, c)
	}
	return comments, total, rows.Err()
}
