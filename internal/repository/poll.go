package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Poll represents a poll attached to a post.
type Poll struct {
	ID        string     `json:"id"`
	PostID    string     `json:"post_id"`
	Deadline  *time.Time `json:"deadline,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// PollOption represents a single choice in a poll with its vote count.
type PollOption struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	VoteCount int    `json:"vote_count"`
	SortOrder int    `json:"sort_order"`
}

// PollWithResults combines a poll with its options, total votes, and optionally the user's vote.
type PollWithResults struct {
	Poll
	Options    []PollOption `json:"options"`
	TotalVotes int          `json:"total_votes"`
	UserVote   *string      `json:"user_vote,omitempty"`
}

// PollRepo handles database operations for polls.
type PollRepo struct {
	pool *pgxpool.Pool
}

// NewPollRepo creates a new PollRepo.
func NewPollRepo(pool *pgxpool.Pool) *PollRepo {
	return &PollRepo{pool: pool}
}

// CreatePoll inserts a poll and its options in a transaction.
func (r *PollRepo) CreatePoll(ctx context.Context, postID string, options []string, deadline *time.Time) (*Poll, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var poll Poll
	err = tx.QueryRow(ctx, `
		INSERT INTO polls (post_id, deadline)
		VALUES ($1, $2)
		RETURNING id, post_id, deadline, created_at`,
		postID, deadline,
	).Scan(&poll.ID, &poll.PostID, &poll.Deadline, &poll.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert poll: %w", err)
	}

	for i, text := range options {
		_, err = tx.Exec(ctx, `
			INSERT INTO poll_options (poll_id, text, sort_order)
			VALUES ($1, $2, $3)`,
			poll.ID, text, i,
		)
		if err != nil {
			return nil, fmt.Errorf("insert poll option %d: %w", i, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return &poll, nil
}

// GetByPostID returns the poll for a given post with options and vote counts.
func (r *PollRepo) GetByPostID(ctx context.Context, postID string) (*PollWithResults, error) {
	var result PollWithResults
	err := r.pool.QueryRow(ctx, `
		SELECT id, post_id, deadline, created_at
		FROM polls WHERE post_id = $1`,
		postID,
	).Scan(&result.ID, &result.PostID, &result.Deadline, &result.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get poll: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT po.id, po.text, po.sort_order,
			COALESCE((SELECT COUNT(*) FROM poll_votes pv WHERE pv.option_id = po.id), 0) AS vote_count
		FROM poll_options po
		WHERE po.poll_id = $1
		ORDER BY po.sort_order`,
		result.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("get poll options: %w", err)
	}
	defer rows.Close()

	var totalVotes int
	for rows.Next() {
		var opt PollOption
		if err := rows.Scan(&opt.ID, &opt.Text, &opt.SortOrder, &opt.VoteCount); err != nil {
			return nil, fmt.Errorf("scan poll option: %w", err)
		}
		totalVotes += opt.VoteCount
		result.Options = append(result.Options, opt)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate poll options: %w", rows.Err())
	}

	result.TotalVotes = totalVotes
	return &result, nil
}

// Vote casts a vote on a poll. The unique constraint on (poll_id, participant_id)
// prevents double-voting.
func (r *PollRepo) Vote(ctx context.Context, pollID, optionID, participantID string) error {
	// ON CONFLICT keeps this idempotent under a concurrent double-vote race:
	// the handler pre-checks GetUserVote, but two simultaneous requests can
	// both pass that check. Without ON CONFLICT the second INSERT hit the
	// unique (poll_id, participant_id) constraint and surfaced as a 500;
	// now the first vote stands and the duplicate is a silent no-op.
	_, err := r.pool.Exec(ctx, `
		INSERT INTO poll_votes (poll_id, option_id, participant_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (poll_id, participant_id) DO NOTHING`,
		pollID, optionID, participantID,
	)
	if err != nil {
		return fmt.Errorf("cast poll vote: %w", err)
	}
	return nil
}

// GetUserVote returns the option_id the participant voted for, or nil if they haven't voted.
func (r *PollRepo) GetUserVote(ctx context.Context, pollID, participantID string) (*string, error) {
	var optionID string
	err := r.pool.QueryRow(ctx, `
		SELECT option_id FROM poll_votes
		WHERE poll_id = $1 AND participant_id = $2`,
		pollID, participantID,
	).Scan(&optionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get user poll vote: %w", err)
	}
	return &optionID, nil
}
