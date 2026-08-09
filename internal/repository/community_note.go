// Package repository — Community Notes storage.
//
// Notes attach to posts. Other participants rate each note as helpful
// or not-helpful. Once enough distinct raters find a note helpful (and
// more do than don't), status flips to 'shown' and the note surfaces
// publicly on the post. Thresholds are intentionally simple for v1;
// bridging-based ranking is a follow-up.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// NoteShowMinHelpful is the absolute number of helpful ratings
	// required before a note can be shown. A high enough floor that
	// one friend can't publish your note for you.
	NoteShowMinHelpful = 3
	// NoteShowHelpfulRatio is the minimum ratio of helpful to
	// not-helpful ratings. 2.0 means helpful must be at least double
	// not-helpful (e.g. 4 helpful / 2 not-helpful qualifies;
	// 3 helpful / 2 not-helpful does not).
	NoteShowHelpfulRatio = 2.0
)

type CommunityNote struct {
	ID              string    `json:"id"`
	PostID          string    `json:"post_id"`
	AuthorID        string    `json:"author_id"`
	Body            string    `json:"body"`
	Sources         []string  `json:"sources"`
	Status          string    `json:"status"` // pending | shown | hidden
	HelpfulCount    int       `json:"helpful_count"`
	NotHelpfulCount int       `json:"not_helpful_count"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	// Fields joined from participants for the list endpoint.
	AuthorName string `json:"author_name,omitempty"`
	AuthorType string `json:"author_type,omitempty"`

	// Viewer-specific — only populated when ListByPost is called with
	// a known viewer.
	MyRating *string `json:"my_rating,omitempty"`
}

type CommunityNoteRepo struct {
	pool *pgxpool.Pool
}

func NewCommunityNoteRepo(pool *pgxpool.Pool) *CommunityNoteRepo {
	return &CommunityNoteRepo{pool: pool}
}

// Create inserts a new note. Sources must be non-empty (enforced by
// CHECK constraint too, but we fail early with a friendlier message).
func (r *CommunityNoteRepo) Create(ctx context.Context, postID, authorID, body string, sources []string) (*CommunityNote, error) {
	if len(sources) == 0 {
		return nil, errors.New("at least one source is required")
	}
	var n CommunityNote
	err := r.pool.QueryRow(ctx, `
		INSERT INTO community_notes (post_id, author_id, body, sources)
		VALUES ($1, $2, $3, $4)
		RETURNING id, post_id, author_id, body, sources, status,
		          helpful_count, not_helpful_count, created_at, updated_at`,
		postID, authorID, body, sources,
	).Scan(
		&n.ID, &n.PostID, &n.AuthorID, &n.Body, &n.Sources, &n.Status,
		&n.HelpfulCount, &n.NotHelpfulCount, &n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert community note: %w", err)
	}
	return &n, nil
}

// ListByPost returns notes on a given post with author metadata and —
// if viewerID is non-empty — the viewer's own rating for each note.
// Ordered: shown → pending → hidden, newest first within each group.
func (r *CommunityNoteRepo) ListByPost(ctx context.Context, postID, viewerID string) ([]CommunityNote, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT n.id, n.post_id, n.author_id, n.body, n.sources, n.status,
		       n.helpful_count, n.not_helpful_count, n.created_at, n.updated_at,
		       p.display_name, p.type,
		       mr.rating
		FROM community_notes n
		JOIN participants p ON p.id = n.author_id
		LEFT JOIN community_note_ratings mr
			ON mr.note_id = n.id AND mr.rater_id = NULLIF($2, '')::uuid
		WHERE n.post_id = $1
		ORDER BY
			CASE n.status
				WHEN 'shown'   THEN 0
				WHEN 'pending' THEN 1
				ELSE 2
			END,
			n.created_at DESC`,
		postID, viewerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list notes by post: %w", err)
	}
	defer rows.Close()

	var out []CommunityNote
	for rows.Next() {
		var n CommunityNote
		var rating *string
		if err := rows.Scan(
			&n.ID, &n.PostID, &n.AuthorID, &n.Body, &n.Sources, &n.Status,
			&n.HelpfulCount, &n.NotHelpfulCount, &n.CreatedAt, &n.UpdatedAt,
			&n.AuthorName, &n.AuthorType, &rating,
		); err != nil {
			return nil, fmt.Errorf("scan note: %w", err)
		}
		n.MyRating = rating
		out = append(out, n)
	}
	return out, rows.Err()
}

// GetAuthor returns just the author_id of a note — used to block a
// user from rating their own note.
func (r *CommunityNoteRepo) GetAuthor(ctx context.Context, noteID string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`SELECT author_id FROM community_notes WHERE id = $1`, noteID,
	).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errors.New("note not found")
		}
		return "", fmt.Errorf("get note author: %w", err)
	}
	return id, nil
}

// Rate upserts a rater's rating, recounts the cached helpful /
// not-helpful columns on the note, and re-evaluates status against
// the publish threshold. All in one transaction so counts and status
// can't drift out of sync.
func (r *CommunityNoteRepo) Rate(ctx context.Context, noteID, raterID, rating string) (*CommunityNote, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO community_note_ratings (note_id, rater_id, rating)
		VALUES ($1, $2, $3)
		ON CONFLICT (note_id, rater_id) DO UPDATE SET rating = EXCLUDED.rating`,
		noteID, raterID, rating,
	); err != nil {
		return nil, fmt.Errorf("upsert rating: %w", err)
	}

	var n CommunityNote
	err = tx.QueryRow(ctx, `
		WITH counts AS (
			SELECT
				COUNT(*) FILTER (WHERE rating = 'helpful')     AS helpful,
				COUNT(*) FILTER (WHERE rating = 'not_helpful') AS not_helpful
			FROM community_note_ratings
			WHERE note_id = $1
		)
		UPDATE community_notes n
		SET helpful_count = counts.helpful,
		    not_helpful_count = counts.not_helpful,
		    status = CASE
		        WHEN status = 'hidden' THEN 'hidden'
		        WHEN counts.helpful >= $2
		         AND counts.helpful >= counts.not_helpful * $3
		            THEN 'shown'
		        ELSE 'pending'
		    END,
		    updated_at = NOW()
		FROM counts
		WHERE n.id = $1
		RETURNING n.id, n.post_id, n.author_id, n.body, n.sources, n.status,
		          n.helpful_count, n.not_helpful_count, n.created_at, n.updated_at`,
		noteID, NoteShowMinHelpful, NoteShowHelpfulRatio,
	).Scan(
		&n.ID, &n.PostID, &n.AuthorID, &n.Body, &n.Sources, &n.Status,
		&n.HelpfulCount, &n.NotHelpfulCount, &n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("recompute note status: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return &n, nil
}
