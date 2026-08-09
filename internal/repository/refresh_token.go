package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/models"
)

// RefreshTokenRepo handles database operations for refresh tokens.
type RefreshTokenRepo struct {
	pool *pgxpool.Pool
}

// NewRefreshTokenRepo creates a new RefreshTokenRepo.
func NewRefreshTokenRepo(pool *pgxpool.Pool) *RefreshTokenRepo {
	return &RefreshTokenRepo{pool: pool}
}

// Create stores a new refresh token for a participant. tokenHash is the
// bcrypt hash (verified after lookup); lookupHash is the indexed SHA-256
// digest used to find the row in O(1) on refresh.
func (r *RefreshTokenRepo) Create(ctx context.Context, participantID, tokenHash, lookupHash string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (participant_id, token_hash, lookup_hash, expires_at)
		VALUES ($1, $2, $3, $4)`,
		participantID, tokenHash, lookupHash, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("insert refresh_token: %w", err)
	}
	return nil
}

// FindByLookupHash returns the refresh-token row matching the given indexed
// lookup hash, regardless of revoked/expired status, so the caller can both
// validate a live token and detect reuse of an already-revoked one (token
// theft). Returns pgx.ErrNoRows when no such token exists.
func (r *RefreshTokenRepo) FindByLookupHash(ctx context.Context, lookupHash string) (*models.RefreshToken, error) {
	var t models.RefreshToken
	err := r.pool.QueryRow(ctx, `
		SELECT id, participant_id, token_hash, expires_at, created_at, revoked_at
		FROM refresh_tokens
		WHERE lookup_hash = $1`,
		lookupHash,
	).Scan(&t.ID, &t.ParticipantID, &t.TokenHash, &t.ExpiresAt, &t.CreatedAt, &t.RevokedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// FindValid returns all non-revoked, non-expired refresh tokens for the given participant.
func (r *RefreshTokenRepo) FindValid(ctx context.Context, participantID string) ([]models.RefreshToken, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, participant_id, token_hash, expires_at, created_at, revoked_at
		FROM refresh_tokens
		WHERE participant_id = $1
		  AND revoked_at IS NULL
		  AND expires_at > NOW()
		ORDER BY created_at DESC`,
		participantID,
	)
	if err != nil {
		return nil, fmt.Errorf("find valid refresh tokens: %w", err)
	}
	defer rows.Close()

	var tokens []models.RefreshToken
	for rows.Next() {
		var t models.RefreshToken
		if err := rows.Scan(&t.ID, &t.ParticipantID, &t.TokenHash, &t.ExpiresAt, &t.CreatedAt, &t.RevokedAt); err != nil {
			return nil, fmt.Errorf("scanning refresh_token row: %w", err)
		}
		tokens = append(tokens, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating refresh_token rows: %w", err)
	}

	return tokens, nil
}

// Revoke marks a specific refresh token as revoked.
func (r *RefreshTokenRepo) Revoke(ctx context.Context, tokenID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = NOW() WHERE id = $1`,
		tokenID,
	)
	if err != nil {
		return fmt.Errorf("revoke refresh_token: %w", err)
	}
	return nil
}

// RevokeAll marks all refresh tokens for a participant as revoked (logout from all devices).
func (r *RefreshTokenRepo) RevokeAll(ctx context.Context, participantID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = NOW()
		WHERE participant_id = $1 AND revoked_at IS NULL`,
		participantID,
	)
	if err != nil {
		return fmt.Errorf("revoke all refresh_tokens: %w", err)
	}
	return nil
}

// CleanExpired deletes refresh tokens that have expired (background cleanup).
func (r *RefreshTokenRepo) CleanExpired(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM refresh_tokens WHERE expires_at < NOW()`)
	if err != nil {
		return fmt.Errorf("clean expired refresh_tokens: %w", err)
	}
	return nil
}
