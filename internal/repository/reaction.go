package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ReactionRepo handles database operations for comment reactions.
type ReactionRepo struct {
	pool *pgxpool.Pool
}

// NewReactionRepo creates a new ReactionRepo.
func NewReactionRepo(pool *pgxpool.Pool) *ReactionRepo {
	return &ReactionRepo{pool: pool}
}

// Toggle adds a reaction if not present, removes it if already present.
// Returns true if the reaction was added, false if it was removed.
func (r *ReactionRepo) Toggle(ctx context.Context, commentID, participantID, reactionType string) (bool, error) {
	// Try to delete first
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM reactions WHERE comment_id = $1 AND participant_id = $2 AND reaction_type = $3`,
		commentID, participantID, reactionType)
	if err != nil {
		return false, fmt.Errorf("delete reaction: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return false, nil // removed
	}

	// Not found — insert
	_, err = r.pool.Exec(ctx,
		`INSERT INTO reactions (comment_id, participant_id, reaction_type) VALUES ($1, $2, $3)`,
		commentID, participantID, reactionType)
	if err != nil {
		return false, fmt.Errorf("insert reaction: %w", err)
	}
	return true, nil
}

// CountsByComment returns reaction counts for a comment keyed by reaction type.
func (r *ReactionRepo) CountsByComment(ctx context.Context, commentID string) (map[string]int, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT reaction_type, COUNT(*) FROM reactions WHERE comment_id = $1 GROUP BY reaction_type`,
		commentID)
	if err != nil {
		return nil, fmt.Errorf("count reactions: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var rt string
		var count int
		if err := rows.Scan(&rt, &count); err != nil {
			return nil, err
		}
		counts[rt] = count
	}
	return counts, rows.Err()
}

// ToggleOnPost mirrors Toggle but for post-scoped reactions. Same
// upsert-as-toggle semantics: insert on first call, delete on second.
// Constraint: a row must have exactly post_id OR comment_id, never
// both — enforced by the table CHECK.
func (r *ReactionRepo) ToggleOnPost(ctx context.Context, postID, participantID, reactionType string) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM reactions WHERE post_id = $1 AND participant_id = $2 AND reaction_type = $3`,
		postID, participantID, reactionType)
	if err != nil {
		return false, fmt.Errorf("delete post reaction: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return false, nil
	}

	_, err = r.pool.Exec(ctx,
		`INSERT INTO reactions (post_id, participant_id, reaction_type) VALUES ($1, $2, $3)`,
		postID, participantID, reactionType)
	if err != nil {
		return false, fmt.Errorf("insert post reaction: %w", err)
	}
	return true, nil
}

// CountsByPost returns reaction counts for a post keyed by reaction type.
func (r *ReactionRepo) CountsByPost(ctx context.Context, postID string) (map[string]int, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT reaction_type, COUNT(*) FROM reactions WHERE post_id = $1 GROUP BY reaction_type`,
		postID)
	if err != nil {
		return nil, fmt.Errorf("count post reactions: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var rt string
		var count int
		if err := rows.Scan(&rt, &count); err != nil {
			return nil, err
		}
		counts[rt] = count
	}
	return counts, rows.Err()
}

// MyReactionsByPost returns the set of reaction types a specific
// participant has placed on a post — used to render the user's own
// reaction state in the UI without a second trip.
func (r *ReactionRepo) MyReactionsByPost(ctx context.Context, postID, participantID string) (map[string]bool, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT reaction_type FROM reactions WHERE post_id = $1 AND participant_id = $2`,
		postID, participantID)
	if err != nil {
		return nil, fmt.Errorf("my post reactions: %w", err)
	}
	defer rows.Close()

	out := make(map[string]bool)
	for rows.Next() {
		var rt string
		if err := rows.Scan(&rt); err != nil {
			return nil, err
		}
		out[rt] = true
	}
	return out, rows.Err()
}
