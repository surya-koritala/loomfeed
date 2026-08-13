package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/models"
)

// Verifier represents a human who verified a post.
type Verifier struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

// VerificationRepo handles database operations for human verifications.
type VerificationRepo struct {
	pool *pgxpool.Pool
}

// NewVerificationRepo creates a new VerificationRepo.
func NewVerificationRepo(pool *pgxpool.Pool) *VerificationRepo {
	return &VerificationRepo{pool: pool}
}

// Verify inserts a verification and increments the post's count atomically.
// It returns the transaction-consistent post snapshot when this verification
// published a gate-held agent post, otherwise nil.
func (r *VerificationRepo) Verify(ctx context.Context, postID, verifierID string) (*models.PostWithAuthor, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialize verifications for this post so count materialization and the
	// quarantined -> public transition stay correct under concurrent verifiers.
	var wasQuarantined bool
	if err := tx.QueryRow(ctx, `SELECT quarantined FROM posts WHERE id = $1 FOR UPDATE`, postID).Scan(&wasQuarantined); err != nil {
		return nil, fmt.Errorf("lock post for verification: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO human_verifications (post_id, verifier_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`,
		postID, verifierID)
	if err != nil {
		return nil, fmt.Errorf("insert verification: %w", err)
	}

	var nowQuarantined bool
	err = tx.QueryRow(ctx, `
		UPDATE posts
		SET human_verification_count = (
			SELECT COUNT(*) FROM human_verifications WHERE post_id = $1
		),
		quarantined = CASE
			WHEN author_type = 'agent' AND EXISTS (
				SELECT 1 FROM quality_gates q
				WHERE q.community_id = posts.community_id
				  AND q.require_human_verification = TRUE
			) THEN FALSE
			ELSE quarantined
		END
		WHERE id = $1
		RETURNING quarantined`, postID).Scan(&nowQuarantined)
	if err != nil {
		return nil, fmt.Errorf("update verification count: %w", err)
	}
	var published *models.PostWithAuthor
	if wasQuarantined && !nowQuarantined {
		row := tx.QueryRow(ctx, postJoinSelect+` WHERE p.id = $1 AND p.deleted_at IS NULL`, postID)
		post, err := scanPostWithAuthor(row)
		if err != nil {
			return nil, fmt.Errorf("read verified published post: %w", err)
		}
		published = &post
		if _, err := enqueueWebhookEvent(ctx, tx, "post.created", map[string]any{
			"post_id": post.ID, "community_id": post.CommunityID,
			"author_id": post.AuthorID, "author_type": post.AuthorType,
			"title": post.Title, "post_type": post.PostType,
			"tags": post.Tags, "created_at": post.CreatedAt,
		}); err != nil {
			return nil, fmt.Errorf("enqueue verified post.created: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return published, nil
}

// Unverify removes a verification and decrements the post's count atomically.
func (r *VerificationRepo) Unverify(ctx context.Context, postID, verifierID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Match Verify's lock order to avoid post/verification-row deadlocks.
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT TRUE FROM posts WHERE id = $1 FOR UPDATE`, postID).Scan(&exists); err != nil {
		return fmt.Errorf("lock post for unverification: %w", err)
	}

	_, err = tx.Exec(ctx, `
		DELETE FROM human_verifications
		WHERE post_id = $1 AND verifier_id = $2`,
		postID, verifierID)
	if err != nil {
		return fmt.Errorf("delete verification: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE posts
		SET human_verification_count = (
			SELECT COUNT(*) FROM human_verifications WHERE post_id = $1
		),
		quarantined = CASE
			WHEN author_type = 'agent'
			 AND NOT EXISTS (SELECT 1 FROM human_verifications WHERE post_id = $1)
			 AND EXISTS (
				SELECT 1 FROM quality_gates q
				WHERE q.community_id = posts.community_id
				  AND q.require_human_verification = TRUE
			 ) THEN TRUE
			ELSE quarantined
		END
		WHERE id = $1`,
		postID)
	if err != nil {
		return fmt.Errorf("update verification count: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// HasVerified checks if a specific human has verified a post.
func (r *VerificationRepo) HasVerified(ctx context.Context, postID, verifierID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM human_verifications
			WHERE post_id = $1 AND verifier_id = $2
		)`,
		postID, verifierID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check verification: %w", err)
	}
	return exists, nil
}

// GetVerifiers returns the list of humans who verified a post.
func (r *VerificationRepo) GetVerifiers(ctx context.Context, postID string) ([]Verifier, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT hv.verifier_id, COALESCE(p.display_name, '') as display_name, hv.created_at
		FROM human_verifications hv
		LEFT JOIN participants p ON p.id = hv.verifier_id
		WHERE hv.post_id = $1
		ORDER BY hv.created_at ASC`,
		postID)
	if err != nil {
		return nil, fmt.Errorf("list verifiers: %w", err)
	}
	defer rows.Close()

	var verifiers []Verifier
	for rows.Next() {
		var v Verifier
		if err := rows.Scan(&v.ID, &v.DisplayName, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan verifier: %w", err)
		}
		verifiers = append(verifiers, v)
	}
	return verifiers, rows.Err()
}

// GetCount returns the number of human verifications for a post.
func (r *VerificationRepo) GetCount(ctx context.Context, postID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM human_verifications WHERE post_id = $1`,
		postID).Scan(&count)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("count verifications: %w", err)
	}
	return count, nil
}
