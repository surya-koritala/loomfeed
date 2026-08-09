package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Revision represents a historical version of a post or comment.
type Revision struct {
	ID             string         `json:"id"`
	ContentID      string         `json:"content_id"`
	ContentType    string         `json:"content_type"`
	RevisionNumber int            `json:"revision_number"`
	Title          string         `json:"title,omitempty"`
	Body           string         `json:"body"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	CreatedAt      string         `json:"created_at"`
}

// RevisionRepo handles database operations for revisions.
type RevisionRepo struct {
	pool *pgxpool.Pool
}

// NewRevisionRepo creates a new RevisionRepo.
func NewRevisionRepo(pool *pgxpool.Pool) *RevisionRepo {
	return &RevisionRepo{pool: pool}
}

// Create saves a new revision for the given content, auto-numbering it.
func (r *RevisionRepo) Create(ctx context.Context, contentID, contentType, title, body string, metadata map[string]any) error {
	// Get next revision number
	var nextNum int
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(revision_number), 0) + 1 FROM revisions WHERE content_id = $1 AND content_type = $2`,
		contentID, contentType).Scan(&nextNum)
	if err != nil {
		return fmt.Errorf("get next revision: %w", err)
	}

	_, err = r.pool.Exec(ctx,
		`INSERT INTO revisions (content_id, content_type, revision_number, title, body, metadata)
         VALUES ($1, $2, $3, $4, $5, $6)`,
		contentID, contentType, nextNum, title, body, metadata)
	if err != nil {
		return fmt.Errorf("insert revision: %w", err)
	}
	return nil
}

// ListByContent returns all revisions for a content item, newest first.
func (r *RevisionRepo) ListByContent(ctx context.Context, contentID, contentType string) ([]Revision, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, content_id, content_type, revision_number, COALESCE(title, ''), body, created_at
         FROM revisions WHERE content_id = $1 AND content_type = $2
         ORDER BY revision_number DESC`,
		contentID, contentType)
	if err != nil {
		return nil, fmt.Errorf("list revisions: %w", err)
	}
	defer rows.Close()

	var revisions []Revision
	for rows.Next() {
		var rev Revision
		if err := rows.Scan(&rev.ID, &rev.ContentID, &rev.ContentType, &rev.RevisionNumber, &rev.Title, &rev.Body, &rev.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan revision: %w", err)
		}
		revisions = append(revisions, rev)
	}
	return revisions, rows.Err()
}
