package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostClaim is a single factual claim extracted (or authored) from a post.
// Each claim can carry one or more ClaimCitation rows linking to supporting
// or contradicting sources — enabling claim-level provenance rather than
// post-level only.
type PostClaim struct {
	ID        string          `json:"id"`
	PostID    string          `json:"post_id"`
	ClaimText string          `json:"claim_text"`
	Position  int             `json:"position"`
	CreatedAt time.Time       `json:"created_at"`
	Citations []ClaimCitation `json:"citations"`
}

type ClaimCitation struct {
	ID          string    `json:"id"`
	ClaimID     string    `json:"claim_id"`
	SourceURL   string    `json:"source_url"`
	SourceTitle *string   `json:"source_title,omitempty"`
	Quote       *string   `json:"quote,omitempty"`
	Relation    string    `json:"relation"`
	Confidence  *float64  `json:"confidence,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type PostClaimInput struct {
	ClaimText string               `json:"claim_text"`
	Position  int                  `json:"position"`
	Citations []ClaimCitationInput `json:"citations"`
}

type ClaimCitationInput struct {
	SourceURL   string   `json:"source_url"`
	SourceTitle *string  `json:"source_title,omitempty"`
	Quote       *string  `json:"quote,omitempty"`
	Relation    string   `json:"relation"`
	Confidence  *float64 `json:"confidence,omitempty"`
}

type PostClaimRepo struct {
	pool *pgxpool.Pool
}

func NewPostClaimRepo(pool *pgxpool.Pool) *PostClaimRepo {
	return &PostClaimRepo{pool: pool}
}

// ListByPost returns all live claims for a post, each with its citations.
func (r *PostClaimRepo) ListByPost(ctx context.Context, postID string) ([]PostClaim, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, post_id, claim_text, position, created_at
		FROM post_claims
		WHERE post_id = $1 AND deleted_at IS NULL
		ORDER BY position ASC, created_at ASC`, postID)
	if err != nil {
		return nil, fmt.Errorf("list post claims: %w", err)
	}
	defer rows.Close()

	claims := []PostClaim{}
	ids := []string{}
	idx := map[string]int{}
	for rows.Next() {
		var c PostClaim
		if err := rows.Scan(&c.ID, &c.PostID, &c.ClaimText, &c.Position, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan post claim: %w", err)
		}
		c.Citations = []ClaimCitation{}
		idx[c.ID] = len(claims)
		ids = append(ids, c.ID)
		claims = append(claims, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return claims, nil
	}

	citeRows, err := r.pool.Query(ctx, `
		SELECT id, claim_id, source_url, source_title, quote, relation, confidence, created_at
		FROM claim_citations
		WHERE claim_id = ANY($1)
		ORDER BY created_at ASC`, ids)
	if err != nil {
		return nil, fmt.Errorf("list claim citations: %w", err)
	}
	defer citeRows.Close()

	for citeRows.Next() {
		var c ClaimCitation
		if err := citeRows.Scan(&c.ID, &c.ClaimID, &c.SourceURL, &c.SourceTitle, &c.Quote, &c.Relation, &c.Confidence, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan claim citation: %w", err)
		}
		if i, ok := idx[c.ClaimID]; ok {
			claims[i].Citations = append(claims[i].Citations, c)
		}
	}
	return claims, citeRows.Err()
}

// ReplaceAll soft-deletes existing claims for the post and inserts the new set
// in a single transaction. Used when an author updates the claim list for a post.
func (r *PostClaimRepo) ReplaceAll(ctx context.Context, postID string, inputs []PostClaimInput) ([]PostClaim, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE post_claims SET deleted_at = NOW()
		WHERE post_id = $1 AND deleted_at IS NULL`, postID); err != nil {
		return nil, fmt.Errorf("soft-delete existing claims: %w", err)
	}

	out := make([]PostClaim, 0, len(inputs))
	for i, in := range inputs {
		var c PostClaim
		pos := in.Position
		if pos == 0 {
			pos = i
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO post_claims (post_id, claim_text, position)
			VALUES ($1, $2, $3)
			RETURNING id, post_id, claim_text, position, created_at`,
			postID, in.ClaimText, pos,
		).Scan(&c.ID, &c.PostID, &c.ClaimText, &c.Position, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("insert claim: %w", err)
		}
		c.Citations = make([]ClaimCitation, 0, len(in.Citations))
		for _, cit := range in.Citations {
			rel := cit.Relation
			if rel == "" {
				rel = "supports"
			}
			var row ClaimCitation
			if err := tx.QueryRow(ctx, `
				INSERT INTO claim_citations (claim_id, source_url, source_title, quote, relation, confidence)
				VALUES ($1, $2, $3, $4, $5, $6)
				RETURNING id, claim_id, source_url, source_title, quote, relation, confidence, created_at`,
				c.ID, cit.SourceURL, cit.SourceTitle, cit.Quote, rel, cit.Confidence,
			).Scan(&row.ID, &row.ClaimID, &row.SourceURL, &row.SourceTitle, &row.Quote, &row.Relation, &row.Confidence, &row.CreatedAt); err != nil {
				return nil, fmt.Errorf("insert citation: %w", err)
			}
			c.Citations = append(c.Citations, row)
		}
		out = append(out, c)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return out, nil
}
