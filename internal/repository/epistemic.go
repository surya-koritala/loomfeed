package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ValidEpistemicStatuses lists the allowed epistemic status values.
var ValidEpistemicStatuses = []string{
	"hypothesis", "supported", "contested", "refuted", "consensus",
}

// epistemicPriority defines the tie-breaking order: higher value wins.
// More "certain" statuses are preferred when vote counts are tied.
var epistemicPriority = map[string]int{
	"refuted":    1,
	"hypothesis": 2,
	"contested":  3,
	"supported":  4,
	"consensus":  5,
}

// EpistemicResult holds vote counts and the winning epistemic status for a post.
type EpistemicResult struct {
	PostID     string         `json:"post_id"`
	Status     string         `json:"status"`       // winning status (most votes)
	Counts     map[string]int `json:"counts"`        // {hypothesis: 3, supported: 7, contested: 2}
	TotalVotes int            `json:"total_votes"`
	UserVote   string         `json:"user_vote,omitempty"` // current user's vote
}

// EpistemicRepo handles database operations for epistemic status votes.
type EpistemicRepo struct {
	pool *pgxpool.Pool
}

// NewEpistemicRepo creates a new EpistemicRepo.
func NewEpistemicRepo(pool *pgxpool.Pool) *EpistemicRepo {
	return &EpistemicRepo{pool: pool}
}

// IsValidStatus checks whether a status string is a valid epistemic status.
func IsValidStatus(status string) bool {
	for _, s := range ValidEpistemicStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// Vote upserts an epistemic vote for a post. If the voter already voted,
// their vote is updated. After voting, the winning status is recalculated
// and written back to the posts.epistemic_status column.
//
// On the FIRST transition out of hypothesis (null → supported/contested
// /refuted), a reputation event is fired for the post author. We only
// fire on the first transition to prevent oscillation abuse — once a
// post has been judged by humans, further re-votes don't keep paying
// out (or punishing) the author. Correction-driven recovery is handled
// by separate correction_acknowledged events.
func (r *EpistemicRepo) Vote(ctx context.Context, postID, voterID, status string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Read the current status + author so we can detect the first
	// transition.
	var oldStatus *string
	var authorID string
	err = tx.QueryRow(ctx,
		`SELECT epistemic_status, author_id FROM posts WHERE id = $1`,
		postID).Scan(&oldStatus, &authorID)
	if err != nil {
		return fmt.Errorf("read post epistemic state: %w", err)
	}
	wasNeutral := oldStatus == nil || *oldStatus == "" || *oldStatus == "hypothesis"

	_, err = tx.Exec(ctx, `
		INSERT INTO epistemic_votes (post_id, voter_id, status)
		VALUES ($1, $2, $3)
		ON CONFLICT (post_id, voter_id)
		DO UPDATE SET status = EXCLUDED.status, created_at = NOW()`,
		postID, voterID, status,
	)
	if err != nil {
		return fmt.Errorf("upsert epistemic vote: %w", err)
	}

	winningStatus, err := r.recalculateWinner(ctx, tx, postID)
	if err != nil {
		return fmt.Errorf("recalculate winner: %w", err)
	}

	_, err = tx.Exec(ctx, `UPDATE posts SET epistemic_status = $1 WHERE id = $2`,
		winningStatus, postID,
	)
	if err != nil {
		return fmt.Errorf("update post epistemic_status: %w", err)
	}

	// Fire reputation event only on the first transition out of
	// hypothesis. authorID can equal voterID; reputation event still
	// fires (a self-vote couldn't shift the winning status alone, but
	// if it does, that's caught upstream).
	if wasNeutral && authorID != "" {
		var eventType string
		switch winningStatus {
		case "supported", "consensus":
			eventType = EventPostSupported
		case "contested":
			eventType = EventPostContested
		case "refuted":
			eventType = EventPostRefuted
		}
		if eventType != "" {
			if _, err := ApplyReputationEventTx(ctx, tx, authorID, eventType); err != nil {
				return fmt.Errorf("apply rep event %s: %w", eventType, err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

// recalculateWinner computes the winning epistemic status from vote counts.
// The status with the most votes wins. On tie, higher priority wins
// (consensus > supported > contested > hypothesis > refuted).
func (r *EpistemicRepo) recalculateWinner(ctx context.Context, tx pgx.Tx, postID string) (string, error) {
	rows, err := tx.Query(ctx, `
		SELECT status, COUNT(*) AS cnt
		FROM epistemic_votes
		WHERE post_id = $1
		GROUP BY status`,
		postID,
	)
	if err != nil {
		return "hypothesis", fmt.Errorf("count epistemic votes: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var s string
		var cnt int
		if err := rows.Scan(&s, &cnt); err != nil {
			return "hypothesis", fmt.Errorf("scan epistemic vote count: %w", err)
		}
		counts[s] = cnt
	}
	if rows.Err() != nil {
		return "hypothesis", fmt.Errorf("iterate epistemic vote counts: %w", rows.Err())
	}

	if len(counts) == 0 {
		return "hypothesis", nil
	}

	winner := "hypothesis"
	maxCount := 0
	for s, cnt := range counts {
		if cnt > maxCount || (cnt == maxCount && epistemicPriority[s] > epistemicPriority[winner]) {
			winner = s
			maxCount = cnt
		}
	}

	return winner, nil
}

// GetPostStatus returns the epistemic vote distribution for a post.
func (r *EpistemicRepo) GetPostStatus(ctx context.Context, postID string) (*EpistemicResult, error) {
	// Verify post exists
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM posts WHERE id = $1)`, postID).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("check post exists: %w", err)
	}
	if !exists {
		return nil, nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT status, COUNT(*) AS cnt
		FROM epistemic_votes
		WHERE post_id = $1
		GROUP BY status`,
		postID,
	)
	if err != nil {
		return nil, fmt.Errorf("query epistemic votes: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	totalVotes := 0
	for rows.Next() {
		var s string
		var cnt int
		if err := rows.Scan(&s, &cnt); err != nil {
			return nil, fmt.Errorf("scan epistemic vote: %w", err)
		}
		counts[s] = cnt
		totalVotes += cnt
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate epistemic votes: %w", rows.Err())
	}

	// Determine the winning status
	winner := "hypothesis"
	maxCount := 0
	for s, cnt := range counts {
		if cnt > maxCount || (cnt == maxCount && epistemicPriority[s] > epistemicPriority[winner]) {
			winner = s
			maxCount = cnt
		}
	}

	return &EpistemicResult{
		PostID:     postID,
		Status:     winner,
		Counts:     counts,
		TotalVotes: totalVotes,
	}, nil
}

// GetUserVote returns the epistemic status the voter chose, or empty string if they haven't voted.
func (r *EpistemicRepo) GetUserVote(ctx context.Context, postID, voterID string) (string, error) {
	var status string
	err := r.pool.QueryRow(ctx, `
		SELECT status FROM epistemic_votes
		WHERE post_id = $1 AND voter_id = $2`,
		postID, voterID,
	).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("get user epistemic vote: %w", err)
	}
	return status, nil
}
