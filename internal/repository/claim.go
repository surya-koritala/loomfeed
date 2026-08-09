package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ClaimVerification struct {
	ID         string    `json:"id"`
	CommentID  string    `json:"comment_id"`
	VerifierID string    `json:"verifier_id"`
	ClaimText  string    `json:"claim_text"`
	Status     string    `json:"status"`
	Evidence   *string   `json:"evidence,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type ClaimVerificationWithVerifier struct {
	ClaimVerification
	VerifierName string `json:"verifier_name"`
	VerifierType string `json:"verifier_type"`
}

type ClaimRepo struct {
	pool *pgxpool.Pool
}

func NewClaimRepo(pool *pgxpool.Pool) *ClaimRepo {
	return &ClaimRepo{pool: pool}
}

func (r *ClaimRepo) Create(ctx context.Context, commentID, verifierID, claimText, status string, evidence *string) (*ClaimVerification, error) {
	var cv ClaimVerification
	err := r.pool.QueryRow(ctx, `
		INSERT INTO claim_verifications (comment_id, verifier_id, claim_text, status, evidence)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (comment_id, verifier_id, claim_text) DO UPDATE SET status = $4, evidence = $5
		RETURNING id, comment_id, verifier_id, claim_text, status, evidence, created_at`,
		commentID, verifierID, claimText, status, evidence,
	).Scan(&cv.ID, &cv.CommentID, &cv.VerifierID, &cv.ClaimText, &cv.Status, &cv.Evidence, &cv.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create claim verification: %w", err)
	}
	return &cv, nil
}

func (r *ClaimRepo) ListByComment(ctx context.Context, commentID string) ([]ClaimVerificationWithVerifier, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT cv.id, cv.comment_id, cv.verifier_id, cv.claim_text, cv.status, cv.evidence, cv.created_at,
		       p.display_name, p.type
		FROM claim_verifications cv
		JOIN participants p ON p.id = cv.verifier_id
		WHERE cv.comment_id = $1
		ORDER BY cv.created_at ASC`, commentID)
	if err != nil {
		return nil, fmt.Errorf("list claim verifications: %w", err)
	}
	defer rows.Close()

	var claims []ClaimVerificationWithVerifier
	for rows.Next() {
		var c ClaimVerificationWithVerifier
		if err := rows.Scan(&c.ID, &c.CommentID, &c.VerifierID, &c.ClaimText, &c.Status, &c.Evidence, &c.CreatedAt, &c.VerifierName, &c.VerifierType); err != nil {
			return nil, fmt.Errorf("scan claim: %w", err)
		}
		claims = append(claims, c)
	}
	return claims, rows.Err()
}
