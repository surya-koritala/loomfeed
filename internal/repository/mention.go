package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Mention represents a mention of a participant in content.
type Mention struct {
	ID          string    `json:"id"`
	ContentID   string    `json:"content_id"`
	ContentType string    `json:"content_type"`
	MentionedID string    `json:"mentioned_id"`
	MentionerID string    `json:"mentioner_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// MentionRepo handles database operations for mentions.
type MentionRepo struct {
	pool *pgxpool.Pool
}

// NewMentionRepo creates a new MentionRepo.
func NewMentionRepo(pool *pgxpool.Pool) *MentionRepo {
	return &MentionRepo{pool: pool}
}

// Create inserts a new mention (INSERT ON CONFLICT DO NOTHING).
func (r *MentionRepo) Create(ctx context.Context, contentID, contentType, mentionedID, mentionerID string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO mentions (content_id, content_type, mentioned_id, mentioner_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (content_id, content_type, mentioned_id) DO NOTHING`,
		contentID, contentType, mentionedID, mentionerID)
	if err != nil {
		return fmt.Errorf("create mention: %w", err)
	}
	return nil
}

// MentionWithContext is the rendering-ready shape returned by
// ListForRecipient — enough for the Mentions tab on a profile to
// show a card per mention without an N+1 follow-up.
type MentionWithContext struct {
	ID            string    `json:"id"`
	ContentID     string    `json:"content_id"`
	ContentType   string    `json:"content_type"`
	MentionerID   string    `json:"mentioner_id"`
	MentionerName string    `json:"mentioner_display_name"`
	MentionerType string    `json:"mentioner_type,omitempty"`
	PostID        string    `json:"post_id,omitempty"`
	PostTitle     string    `json:"post_title,omitempty"`
	PostSlug      string    `json:"post_slug,omitempty"`
	Body          string    `json:"body"` // post or comment body, capped
	CreatedAt     time.Time `json:"created_at"`
}

// ListForRecipient returns mentions where the given participant was
// mentioned, newest first. JOINs with posts/comments + participants
// so the profile page renders without N+1. Soft-deleted content is
// skipped.
func (r *MentionRepo) ListForRecipient(ctx context.Context, recipientID string, limit, offset int) ([]MentionWithContext, int, error) {
	var total int
	_ = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM mentions WHERE mentioned_id = $1`,
		recipientID).Scan(&total)

	rows, err := r.pool.Query(ctx, `
		SELECT
		  m.id,
		  m.content_id,
		  m.content_type,
		  m.mentioner_id,
		  COALESCE(mp.display_name, ''),
		  COALESCE(mp.type::text, ''),
		  COALESCE(p.id::text, ''),
		  COALESCE(p.title, ''),
		  COALESCE(p.slug, ''),
		  COALESCE(LEFT(
		    CASE WHEN m.content_type = 'post' THEN p.body ELSE c.body END,
		    400
		  ), ''),
		  m.created_at
		FROM mentions m
		LEFT JOIN participants mp ON mp.id = m.mentioner_id
		LEFT JOIN posts p
		  ON (m.content_type = 'post' AND p.id = m.content_id)
		  OR (m.content_type = 'comment' AND p.id = c.post_id)
		LEFT JOIN comments c ON m.content_type = 'comment' AND c.id = m.content_id
		WHERE m.mentioned_id = $1
		  AND (m.content_type <> 'post' OR p.deleted_at IS NULL)
		  AND (m.content_type <> 'comment' OR (c.deleted_at IS NULL AND p.deleted_at IS NULL))
		ORDER BY m.created_at DESC
		LIMIT $2 OFFSET $3`,
		recipientID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list mentions for recipient: %w", err)
	}
	defer rows.Close()

	out := make([]MentionWithContext, 0)
	for rows.Next() {
		var m MentionWithContext
		if err := rows.Scan(
			&m.ID, &m.ContentID, &m.ContentType, &m.MentionerID,
			&m.MentionerName, &m.MentionerType,
			&m.PostID, &m.PostTitle, &m.PostSlug, &m.Body,
			&m.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan mention: %w", err)
		}
		out = append(out, m)
	}
	return out, total, rows.Err()
}

// ListByContent returns all mentions for a given content item.
func (r *MentionRepo) ListByContent(ctx context.Context, contentID, contentType string) ([]Mention, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, content_id, content_type, mentioned_id, mentioner_id, created_at
		FROM mentions
		WHERE content_id = $1 AND content_type = $2
		ORDER BY created_at DESC`,
		contentID, contentType)
	if err != nil {
		return nil, fmt.Errorf("list mentions by content: %w", err)
	}
	defer rows.Close()

	var mentions []Mention
	for rows.Next() {
		var m Mention
		if err := rows.Scan(&m.ID, &m.ContentID, &m.ContentType, &m.MentionedID, &m.MentionerID, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan mention: %w", err)
		}
		mentions = append(mentions, m)
	}
	return mentions, rows.Err()
}
