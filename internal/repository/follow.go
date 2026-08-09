package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Follow represents a follow relationship between two participants.
type Follow struct {
	FollowerID string    `json:"follower_id"`
	FollowedID string    `json:"followed_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// FollowRepo handles database operations for follows.
type FollowRepo struct {
	pool *pgxpool.Pool
}

// NewFollowRepo creates a new FollowRepo.
func NewFollowRepo(pool *pgxpool.Pool) *FollowRepo {
	return &FollowRepo{pool: pool}
}

// Follow creates a follow relationship and atomically updates follower/following counts.
func (r *FollowRepo) Follow(ctx context.Context, followerID, followedID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		INSERT INTO follows (follower_id, followed_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`,
		followerID, followedID)
	if err != nil {
		return fmt.Errorf("insert follow: %w", err)
	}

	// Only update counts if a row was actually inserted
	if tag.RowsAffected() > 0 {
		_, err = tx.Exec(ctx, `UPDATE participants SET following_count = following_count + 1 WHERE id = $1`, followerID)
		if err != nil {
			return fmt.Errorf("update following_count: %w", err)
		}
		_, err = tx.Exec(ctx, `UPDATE participants SET follower_count = follower_count + 1 WHERE id = $1`, followedID)
		if err != nil {
			return fmt.Errorf("update follower_count: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// Unfollow removes a follow relationship and atomically updates follower/following counts.
func (r *FollowRepo) Unfollow(ctx context.Context, followerID, followedID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		DELETE FROM follows WHERE follower_id = $1 AND followed_id = $2`,
		followerID, followedID)
	if err != nil {
		return fmt.Errorf("delete follow: %w", err)
	}

	// Only update counts if a row was actually deleted
	if tag.RowsAffected() > 0 {
		_, err = tx.Exec(ctx, `UPDATE participants SET following_count = following_count - 1 WHERE id = $1`, followerID)
		if err != nil {
			return fmt.Errorf("update following_count: %w", err)
		}
		_, err = tx.Exec(ctx, `UPDATE participants SET follower_count = follower_count - 1 WHERE id = $1`, followedID)
		if err != nil {
			return fmt.Errorf("update follower_count: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// IsFollowing checks if followerID is following followedID.
func (r *FollowRepo) IsFollowing(ctx context.Context, followerID, followedID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM follows WHERE follower_id = $1 AND followed_id = $2)`,
		followerID, followedID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("is following: %w", err)
	}
	return exists, nil
}

// ListFollowing returns participants that the given participant is following.
func (r *FollowRepo) ListFollowing(ctx context.Context, participantID string, limit, offset int) ([]Follow, int, error) {
	var total int
	_ = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM follows WHERE follower_id = $1`, participantID).Scan(&total)

	rows, err := r.pool.Query(ctx, `
		SELECT follower_id, followed_id, created_at
		FROM follows
		WHERE follower_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`,
		participantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list following: %w", err)
	}
	defer rows.Close()

	var follows []Follow
	for rows.Next() {
		var f Follow
		if err := rows.Scan(&f.FollowerID, &f.FollowedID, &f.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan follow: %w", err)
		}
		follows = append(follows, f)
	}
	return follows, total, rows.Err()
}

// ListFollowers returns participants that follow the given participant.
func (r *FollowRepo) ListFollowers(ctx context.Context, participantID string, limit, offset int) ([]Follow, int, error) {
	var total int
	_ = r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM follows WHERE followed_id = $1`, participantID).Scan(&total)

	rows, err := r.pool.Query(ctx, `
		SELECT follower_id, followed_id, created_at
		FROM follows
		WHERE followed_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`,
		participantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list followers: %w", err)
	}
	defer rows.Close()

	var follows []Follow
	for rows.Next() {
		var f Follow
		if err := rows.Scan(&f.FollowerID, &f.FollowedID, &f.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan follow: %w", err)
		}
		follows = append(follows, f)
	}
	return follows, total, rows.Err()
}

// FollowedSet returns which of the given participant IDs the follower
// currently follows. Used to annotate post payloads with
// viewer_following in one query instead of N IsFollowing calls.
func (r *FollowRepo) FollowedSet(ctx context.Context, followerID string, ids []string) (map[string]bool, error) {
	out := map[string]bool{}
	if followerID == "" || len(ids) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT followed_id FROM follows WHERE follower_id = $1 AND followed_id = ANY($2)`,
		followerID, ids)
	if err != nil {
		return nil, fmt.Errorf("followed set: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan followed id: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}

// GetFollowingIDs returns the IDs of all participants that the given participant is following.
func (r *FollowRepo) GetFollowingIDs(ctx context.Context, participantID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT followed_id FROM follows WHERE follower_id = $1`,
		participantID)
	if err != nil {
		return nil, fmt.Errorf("get following ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan following id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
