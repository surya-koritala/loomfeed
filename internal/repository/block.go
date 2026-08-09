package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BlockRepo handles participant_blocks.
type BlockRepo struct {
	pool *pgxpool.Pool
}

// NewBlockRepo creates a BlockRepo.
func NewBlockRepo(pool *pgxpool.Pool) *BlockRepo {
	return &BlockRepo{pool: pool}
}

// Block inserts (blocker, blocked). Idempotent: re-blocking is a
// no-op. Self-blocks are rejected by the table CHECK.
func (r *BlockRepo) Block(ctx context.Context, blockerID, blockedID string) error {
	if blockerID == blockedID {
		return fmt.Errorf("cannot block yourself")
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO participant_blocks (blocker_id, blocked_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`,
		blockerID, blockedID)
	if err != nil {
		return fmt.Errorf("insert block: %w", err)
	}
	return nil
}

// Unblock removes (blocker, blocked). No-op if no row exists.
func (r *BlockRepo) Unblock(ctx context.Context, blockerID, blockedID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM participant_blocks WHERE blocker_id = $1 AND blocked_id = $2`,
		blockerID, blockedID)
	if err != nil {
		return fmt.Errorf("delete block: %w", err)
	}
	return nil
}

// IsBlocked reports whether blockerID has blocked blockedID.
func (r *BlockRepo) IsBlocked(ctx context.Context, blockerID, blockedID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM participant_blocks
		   WHERE blocker_id = $1 AND blocked_id = $2)`,
		blockerID, blockedID).Scan(&exists)
	return exists, err
}

// ListBlockedIDs returns the IDs of every participant blocked by
// the given blocker. Used as a sub-query / IN-list filter on feed
// and notification queries.
func (r *BlockRepo) ListBlockedIDs(ctx context.Context, blockerID string) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT blocked_id FROM participant_blocks WHERE blocker_id = $1`,
		blockerID)
	if err != nil {
		return nil, fmt.Errorf("list blocked ids: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan blocked id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// BlockedRow is the rendering-ready shape returned by
// ListBlockedWithDetails — enough for /settings to render the
// "blocked users" list with avatars and timestamps.
type BlockedRow struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	Type        string    `json:"type"`
	BlockedAt   time.Time `json:"blocked_at"`
}

// ListBlockedWithDetails returns every block by blockerID joined
// with the blocked participant's profile fields. One round trip,
// newest blocks first.
func (r *BlockRepo) ListBlockedWithDetails(ctx context.Context, blockerID string) ([]BlockedRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
		  p.id,
		  COALESCE(p.display_name, ''),
		  COALESCE(p.avatar_url, ''),
		  COALESCE(p.type::text, ''),
		  b.created_at
		FROM participant_blocks b
		JOIN participants p ON p.id = b.blocked_id
		WHERE b.blocker_id = $1
		ORDER BY b.created_at DESC`,
		blockerID)
	if err != nil {
		return nil, fmt.Errorf("list blocked with details: %w", err)
	}
	defer rows.Close()
	out := make([]BlockedRow, 0)
	for rows.Next() {
		var b BlockedRow
		if err := rows.Scan(&b.ID, &b.DisplayName, &b.AvatarURL, &b.Type, &b.BlockedAt); err != nil {
			return nil, fmt.Errorf("scan blocked row: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
