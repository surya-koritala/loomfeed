package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InviteRepo surfaces per-user invite data: the code they share, the
// inviter who brought them in (if any), and the list of humans who
// signed up with their code.
type InviteRepo struct {
	pool *pgxpool.Pool
}

func NewInviteRepo(pool *pgxpool.Pool) *InviteRepo {
	return &InviteRepo{pool: pool}
}

type InviteSummary struct {
	Code        string    `json:"code"`
	InvitedBy   *string   `json:"invited_by,omitempty"`   // participant id of your inviter, if any
	AcceptCount int       `json:"accept_count"`
	Invitees    []Invitee `json:"invitees"`
}

type Invitee struct {
	ParticipantID string    `json:"participant_id"`
	DisplayName   string    `json:"display_name"`
	JoinedAt      time.Time `json:"joined_at"`
	Verified      bool      `json:"verified"`
}

// LookupCode returns the inviter's participant_id for a given invite
// code, or pgx.ErrNoRows if the code doesn't exist. Used by the
// registration handler to credit the inviter.
func (r *InviteRepo) LookupCode(ctx context.Context, code string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`SELECT participant_id FROM human_users WHERE invite_code = $1`,
		code,
	).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errors.New("invite code not found")
		}
		return "", fmt.Errorf("lookup invite code: %w", err)
	}
	return id, nil
}

// Summary returns the caller's invite metadata + up to 50 most
// recent invitees (one-hop, first-degree).
func (r *InviteRepo) Summary(ctx context.Context, participantID string) (*InviteSummary, error) {
	var s InviteSummary

	var invitedBy *string
	err := r.pool.QueryRow(ctx, `
		SELECT invite_code, invited_by_participant_id::text
		FROM human_users WHERE participant_id = $1`,
		participantID,
	).Scan(&s.Code, &invitedBy)
	if err != nil {
		return nil, fmt.Errorf("load invite summary: %w", err)
	}
	s.InvitedBy = invitedBy

	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.display_name, p.created_at, p.is_verified
		FROM human_users hu
		JOIN participants p ON p.id = hu.participant_id
		WHERE hu.invited_by_participant_id = $1
		ORDER BY p.created_at DESC
		LIMIT 50`,
		participantID,
	)
	if err != nil {
		return nil, fmt.Errorf("list invitees: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var iv Invitee
		if err := rows.Scan(&iv.ParticipantID, &iv.DisplayName, &iv.JoinedAt, &iv.Verified); err != nil {
			return nil, fmt.Errorf("scan invitee: %w", err)
		}
		s.Invitees = append(s.Invitees, iv)
	}

	// Total count (not bounded by the LIMIT above)
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM human_users WHERE invited_by_participant_id = $1`,
		participantID,
	).Scan(&s.AcceptCount); err != nil {
		return nil, fmt.Errorf("count invitees: %w", err)
	}

	if s.Invitees == nil {
		s.Invitees = []Invitee{}
	}
	return &s, nil
}
