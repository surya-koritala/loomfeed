package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Challenge represents a content challenge / bounty.
type Challenge struct {
	ID                   string     `json:"id"`
	Title                string     `json:"title"`
	Body                 string     `json:"body"`
	CommunityID          string     `json:"community_id"`
	CommunityName        string     `json:"community_name,omitempty"`
	CommunitySlug        string     `json:"community_slug,omitempty"`
	CreatedBy            string     `json:"created_by"`
	CreatedByName        string     `json:"created_by_name,omitempty"`
	Status               string     `json:"status"`
	Deadline             *time.Time `json:"deadline,omitempty"`
	RequiredCapabilities []string   `json:"required_capabilities"`
	WinnerID             *string    `json:"winner_id,omitempty"`
	SubmissionCount      int        `json:"submission_count"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// ChallengeSubmission represents a submission to a challenge.
type ChallengeSubmission struct {
	ID            string    `json:"id"`
	ChallengeID   string    `json:"challenge_id"`
	ParticipantID string    `json:"participant_id"`
	ParticipantName string  `json:"participant_name,omitempty"`
	Body          string    `json:"body"`
	Score         int       `json:"score"`
	IsWinner      bool      `json:"is_winner"`
	CreatedAt     time.Time `json:"created_at"`
}

// ChallengeRepo handles database operations for challenges.
type ChallengeRepo struct {
	pool *pgxpool.Pool
}

// NewChallengeRepo creates a new ChallengeRepo.
func NewChallengeRepo(pool *pgxpool.Pool) *ChallengeRepo {
	return &ChallengeRepo{pool: pool}
}

// Create inserts a new challenge.
func (r *ChallengeRepo) Create(ctx context.Context, title, body, communityID, createdBy string, deadline *time.Time, capabilities []string) (*Challenge, error) {
	if capabilities == nil {
		capabilities = []string{}
	}
	var c Challenge
	err := r.pool.QueryRow(ctx, `
		INSERT INTO challenges (title, body, community_id, created_by, deadline, required_capabilities)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, title, body, community_id, created_by, status, deadline,
		          required_capabilities, winner_id, created_at, updated_at`,
		title, body, communityID, createdBy, deadline, capabilities,
	).Scan(
		&c.ID, &c.Title, &c.Body, &c.CommunityID, &c.CreatedBy, &c.Status,
		&c.Deadline, &c.RequiredCapabilities, &c.WinnerID, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create challenge: %w", err)
	}
	return &c, nil
}

// List returns challenges filtered by status.
func (r *ChallengeRepo) List(ctx context.Context, status string, limit, offset int) ([]Challenge, error) {
	query := `
		SELECT c.id, c.title, c.body, c.community_id,
		       COALESCE(comm.name, '') as community_name,
		       COALESCE(comm.slug, '') as community_slug,
		       c.created_by, COALESCE(p.display_name, '') as created_by_name,
		       c.status, c.deadline, c.required_capabilities, c.winner_id,
		       (SELECT COUNT(*) FROM challenge_submissions cs WHERE cs.challenge_id = c.id) as submission_count,
		       c.created_at, c.updated_at
		FROM challenges c
		LEFT JOIN communities comm ON comm.id = c.community_id
		LEFT JOIN participants p ON p.id = c.created_by`

	args := []any{}
	argIdx := 1

	if status != "" {
		query += fmt.Sprintf(" WHERE c.status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY c.created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list challenges: %w", err)
	}
	defer rows.Close()

	var challenges []Challenge
	for rows.Next() {
		var c Challenge
		if err := rows.Scan(
			&c.ID, &c.Title, &c.Body, &c.CommunityID,
			&c.CommunityName, &c.CommunitySlug,
			&c.CreatedBy, &c.CreatedByName,
			&c.Status, &c.Deadline, &c.RequiredCapabilities, &c.WinnerID,
			&c.SubmissionCount,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan challenge: %w", err)
		}
		challenges = append(challenges, c)
	}
	return challenges, rows.Err()
}

// GetByID returns a challenge by ID with submission count.
func (r *ChallengeRepo) GetByID(ctx context.Context, id string) (*Challenge, error) {
	var c Challenge
	err := r.pool.QueryRow(ctx, `
		SELECT c.id, c.title, c.body, c.community_id,
		       COALESCE(comm.name, '') as community_name,
		       COALESCE(comm.slug, '') as community_slug,
		       c.created_by, COALESCE(p.display_name, '') as created_by_name,
		       c.status, c.deadline, c.required_capabilities, c.winner_id,
		       (SELECT COUNT(*) FROM challenge_submissions cs WHERE cs.challenge_id = c.id) as submission_count,
		       c.created_at, c.updated_at
		FROM challenges c
		LEFT JOIN communities comm ON comm.id = c.community_id
		LEFT JOIN participants p ON p.id = c.created_by
		WHERE c.id = $1`,
		id,
	).Scan(
		&c.ID, &c.Title, &c.Body, &c.CommunityID,
		&c.CommunityName, &c.CommunitySlug,
		&c.CreatedBy, &c.CreatedByName,
		&c.Status, &c.Deadline, &c.RequiredCapabilities, &c.WinnerID,
		&c.SubmissionCount,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get challenge: %w", err)
	}
	return &c, nil
}

// Submit inserts a new submission to a challenge (UNIQUE per participant per challenge).
func (r *ChallengeRepo) Submit(ctx context.Context, challengeID, participantID, body string) (*ChallengeSubmission, error) {
	var s ChallengeSubmission
	err := r.pool.QueryRow(ctx, `
		INSERT INTO challenge_submissions (challenge_id, participant_id, body)
		VALUES ($1, $2, $3)
		RETURNING id, challenge_id, participant_id, body, score, is_winner, created_at`,
		challengeID, participantID, body,
	).Scan(&s.ID, &s.ChallengeID, &s.ParticipantID, &s.Body, &s.Score, &s.IsWinner, &s.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("submit challenge: %w", err)
	}
	return &s, nil
}

// ListSubmissions returns all submissions for a challenge ordered by score DESC.
func (r *ChallengeRepo) ListSubmissions(ctx context.Context, challengeID string) ([]ChallengeSubmission, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT cs.id, cs.challenge_id, cs.participant_id,
		       COALESCE(p.display_name, '') as participant_name,
		       cs.body, cs.score, cs.is_winner, cs.created_at
		FROM challenge_submissions cs
		LEFT JOIN participants p ON p.id = cs.participant_id
		WHERE cs.challenge_id = $1
		ORDER BY cs.score DESC, cs.created_at ASC`,
		challengeID,
	)
	if err != nil {
		return nil, fmt.Errorf("list submissions: %w", err)
	}
	defer rows.Close()

	var subs []ChallengeSubmission
	for rows.Next() {
		var s ChallengeSubmission
		if err := rows.Scan(
			&s.ID, &s.ChallengeID, &s.ParticipantID, &s.ParticipantName,
			&s.Body, &s.Score, &s.IsWinner, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan submission: %w", err)
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

// VoteSubmission increments the score of a submission by 1.
func (r *ChallengeRepo) VoteSubmission(ctx context.Context, submissionID string) (int, error) {
	var newScore int
	err := r.pool.QueryRow(ctx, `
		UPDATE challenge_submissions SET score = score + 1
		WHERE id = $1
		RETURNING score`,
		submissionID,
	).Scan(&newScore)
	if err != nil {
		return 0, fmt.Errorf("vote submission: %w", err)
	}
	return newScore, nil
}

// PickWinner sets is_winner on the submission, updates the challenge winner_id and status to closed.
// Returns the winner's participant ID so the caller can award reputation.
func (r *ChallengeRepo) PickWinner(ctx context.Context, challengeID, submissionID, judgeID string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Verify this submission belongs to the challenge
	var winnerParticipantID string
	err = tx.QueryRow(ctx, `
		SELECT participant_id FROM challenge_submissions
		WHERE id = $1 AND challenge_id = $2`,
		submissionID, challengeID,
	).Scan(&winnerParticipantID)
	if err != nil {
		return "", fmt.Errorf("get submission: %w", err)
	}

	// Mark submission as winner
	_, err = tx.Exec(ctx, `
		UPDATE challenge_submissions SET is_winner = TRUE WHERE id = $1`,
		submissionID)
	if err != nil {
		return "", fmt.Errorf("mark winner submission: %w", err)
	}

	// Update challenge with winner and close it
	_, err = tx.Exec(ctx, `
		UPDATE challenges SET winner_id = $1, status = 'closed', updated_at = NOW()
		WHERE id = $2`,
		winnerParticipantID, challengeID)
	if err != nil {
		return "", fmt.Errorf("update challenge winner: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit tx: %w", err)
	}
	return winnerParticipantID, nil
}
