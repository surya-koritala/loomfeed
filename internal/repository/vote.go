package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/RoamXAI/loomfeed/internal/models"
)

// VoteRepo handles database operations for votes.
type VoteRepo struct {
	pool *pgxpool.Pool
}

// NewVoteRepo creates a new VoteRepo.
func NewVoteRepo(pool *pgxpool.Pool) *VoteRepo {
	return &VoteRepo{pool: pool}
}

// CastVote casts, toggles, or changes a vote in a transaction.
// Returns the new vote_score of the target.
//
// Logic:
//   - If an existing vote with the same direction exists → DELETE (toggle off)
//   - If an existing vote with a different direction exists → UPDATE direction
//   - If no existing vote → INSERT new vote
//
// After mutation, recalculates and updates vote_score on the target post or comment.
//
// Optimised: uses a single SELECT FOR UPDATE + conditional DML + score update
// (3 queries in a transaction, down from 4+).
func (r *VoteRepo) CastVote(ctx context.Context, v *models.Vote) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Step 1: Lookup existing vote (SELECT FOR UPDATE to prevent races)
	var existingID string
	var existingDirection models.VoteDirection
	err = tx.QueryRow(ctx, `
		SELECT id, direction FROM votes
		WHERE target_id = $1 AND target_type = $2 AND voter_id = $3
		FOR UPDATE`,
		v.TargetID, v.TargetType, v.VoterID,
	).Scan(&existingID, &existingDirection)

	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("check existing vote: %w", err)
	}

	// Step 2: Mutate vote — one of delete / update / insert
	if err == nil {
		if existingDirection == v.Direction {
			_, err = tx.Exec(ctx, `DELETE FROM votes WHERE id = $1`, existingID)
			if err != nil {
				return 0, fmt.Errorf("delete vote (toggle off): %w", err)
			}
		} else {
			_, err = tx.Exec(ctx, `UPDATE votes SET direction = $1 WHERE id = $2`, v.Direction, existingID)
			if err != nil {
				return 0, fmt.Errorf("update vote direction: %w", err)
			}
		}
	} else {
		_, err = tx.Exec(ctx, `
			INSERT INTO votes (target_id, target_type, voter_id, voter_type, direction)
			VALUES ($1, $2, $3, $4, $5)`,
			v.TargetID, v.TargetType, v.VoterID, v.VoterType, v.Direction,
		)
		if err != nil {
			return 0, fmt.Errorf("insert vote: %w", err)
		}
	}

	// Step 3: Recalculate vote_score + Wilson counts in a single query for comments,
	// or just vote_score for posts. Two literal queries instead of
	// one fmt.Sprintf'd template — the audit flagged the dynamic SQL
	// even though the table name was internally controlled, so we
	// remove the smell entirely.
	var newScore int
	if v.TargetType == models.TargetComment {
		// Combined: vote_score + upvote_count + downvote_count in one UPDATE
		err = tx.QueryRow(ctx, `
			UPDATE comments
			SET vote_score = COALESCE(
				(SELECT SUM(CASE WHEN direction = 'up' THEN 1 ELSE -1 END)
				 FROM votes WHERE target_id = $1 AND target_type = $2), 0),
			    upvote_count = (SELECT COUNT(*) FROM votes WHERE target_id = $1 AND target_type = 'comment' AND direction = 'up'),
			    downvote_count = (SELECT COUNT(*) FROM votes WHERE target_id = $1 AND target_type = 'comment' AND direction = 'down')
			WHERE id = $1
			RETURNING vote_score`,
			v.TargetID, v.TargetType,
		).Scan(&newScore)
	} else {
		err = tx.QueryRow(ctx, `
			UPDATE posts
			SET vote_score = COALESCE(
				(SELECT SUM(CASE WHEN direction = 'up' THEN 1 ELSE -1 END)
				 FROM votes WHERE target_id = $1 AND target_type = $2), 0)
			WHERE id = $1
			RETURNING vote_score`,
			v.TargetID, v.TargetType,
		).Scan(&newScore)
	}
	if err != nil {
		return 0, fmt.Errorf("recalculate vote_score: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}

	return newScore, nil
}

// CastWithReputation performs the vote and reputation update in a single transaction.
// This merges the previously separate vote + reputation flows (2 transactions, 7+ queries)
// into a single transaction (5 queries). If authorID is empty or equals the voter, the
// reputation step is skipped.
// Also awards a small trust bump (+0.1) to the voter for community participation (Bug 6 fix).
func (r *VoteRepo) CastWithReputation(ctx context.Context, v *models.Vote, authorID, eventType string, scoreDelta float64) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Step 1: Lookup existing vote (SELECT FOR UPDATE to prevent races)
	var existingID string
	var existingDirection models.VoteDirection
	err = tx.QueryRow(ctx, `
		SELECT id, direction FROM votes
		WHERE target_id = $1 AND target_type = $2 AND voter_id = $3
		FOR UPDATE`,
		v.TargetID, v.TargetType, v.VoterID,
	).Scan(&existingID, &existingDirection)

	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("check existing vote: %w", err)
	}

	// Step 2: Mutate vote — one of delete / update / insert
	if err == nil {
		if existingDirection == v.Direction {
			_, err = tx.Exec(ctx, `DELETE FROM votes WHERE id = $1`, existingID)
			if err != nil {
				return 0, fmt.Errorf("delete vote (toggle off): %w", err)
			}
		} else {
			_, err = tx.Exec(ctx, `UPDATE votes SET direction = $1 WHERE id = $2`, v.Direction, existingID)
			if err != nil {
				return 0, fmt.Errorf("update vote direction: %w", err)
			}
		}
	} else {
		_, err = tx.Exec(ctx, `
			INSERT INTO votes (target_id, target_type, voter_id, voter_type, direction)
			VALUES ($1, $2, $3, $4, $5)`,
			v.TargetID, v.TargetType, v.VoterID, v.VoterType, v.Direction,
		)
		if err != nil {
			return 0, fmt.Errorf("insert vote: %w", err)
		}
	}

	// Step 3: Recalculate vote_score. Two literal queries instead of
	// one fmt.Sprintf'd template — see rationale in CastVote above.
	var newScore int
	if v.TargetType == models.TargetComment {
		err = tx.QueryRow(ctx, `
			UPDATE comments
			SET vote_score = COALESCE(
				(SELECT SUM(CASE WHEN direction = 'up' THEN 1 ELSE -1 END)
				 FROM votes WHERE target_id = $1 AND target_type = $2), 0),
			    upvote_count = (SELECT COUNT(*) FROM votes WHERE target_id = $1 AND target_type = 'comment' AND direction = 'up'),
			    downvote_count = (SELECT COUNT(*) FROM votes WHERE target_id = $1 AND target_type = 'comment' AND direction = 'down')
			WHERE id = $1
			RETURNING vote_score`,
			v.TargetID, v.TargetType,
		).Scan(&newScore)
	} else {
		err = tx.QueryRow(ctx, `
			UPDATE posts
			SET vote_score = COALESCE(
				(SELECT SUM(CASE WHEN direction = 'up' THEN 1 ELSE -1 END)
				 FROM votes WHERE target_id = $1 AND target_type = $2), 0)
			WHERE id = $1
			RETURNING vote_score`,
			v.TargetID, v.TargetType,
		).Scan(&newScore)
	}
	if err != nil {
		return 0, fmt.Errorf("recalculate vote_score: %w", err)
	}

	// Step 4: Reputation update for the content author. Uses the
	// shared uncapped formula in ApplyReputationEventTx so vote-driven
	// rep stays consistent with rep from other sources (verifications,
	// refutations, etc.). The legacy scoreDelta arg is ignored — the
	// canonical magnitude is keyed off eventType.
	_ = scoreDelta // legacy arg, kept for signature stability
	if authorID != "" && authorID != v.VoterID {
		if _, err := ApplyReputationEventTx(ctx, tx, authorID, eventType); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}

	return newScore, nil
}

// GetUserVotesForPosts returns a map of post_id → vote direction for the given user.
func (r *VoteRepo) GetUserVotesForPosts(ctx context.Context, voterID string, postIDs []string) (map[string]string, error) {
	if len(postIDs) == 0 {
		return nil, nil
	}

	args := []any{voterID}
	placeholders := make([]string, len(postIDs))
	for i, id := range postIDs {
		args = append(args, id)
		placeholders[i] = fmt.Sprintf("$%d", i+2)
	}

	query := `SELECT target_id, direction FROM votes WHERE voter_id = $1 AND target_type = 'post' AND target_id IN (` + strings.Join(placeholders, ",") + `)`
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get user votes: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var targetID, direction string
		if err := rows.Scan(&targetID, &direction); err != nil {
			return nil, fmt.Errorf("scan vote: %w", err)
		}
		result[targetID] = direction
	}
	return result, rows.Err()
}

// GetUserVotesForComments returns a map of comment_id → vote direction for the given user.
func (r *VoteRepo) GetUserVotesForComments(ctx context.Context, voterID string, commentIDs []string) (map[string]string, error) {
	if len(commentIDs) == 0 {
		return nil, nil
	}

	args := []any{voterID}
	placeholders := make([]string, len(commentIDs))
	for i, id := range commentIDs {
		args = append(args, id)
		placeholders[i] = fmt.Sprintf("$%d", i+2)
	}

	query := `SELECT target_id, direction FROM votes WHERE voter_id = $1 AND target_type = 'comment' AND target_id IN (` + strings.Join(placeholders, ",") + `)`
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get user comment votes: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var targetID, direction string
		if err := rows.Scan(&targetID, &direction); err != nil {
			return nil, fmt.Errorf("scan comment vote: %w", err)
		}
		result[targetID] = direction
	}
	return result, rows.Err()
}
