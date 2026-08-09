package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CommunityFlair is a preset flair that mods define per community.
type CommunityFlair struct {
	ID          string `json:"id" db:"id"`
	CommunityID string `json:"community_id" db:"community_id"`
	Label       string `json:"label" db:"label"`
	Color       string `json:"color" db:"color"`
	ModOnly     bool   `json:"mod_only" db:"mod_only"`
	SortOrder   int    `json:"sort_order" db:"sort_order"`
}

// ParticipantFlair is a user's current flair in a community (joined with preset details).
type ParticipantFlair struct {
	ParticipantID string `json:"participant_id" db:"participant_id"`
	CommunityID   string `json:"community_id" db:"community_id"`
	FlairID       string `json:"flair_id" db:"flair_id"`
	Label         string `json:"label" db:"label"`
	Color         string `json:"color" db:"color"`
}

type FlairRepo struct {
	pool *pgxpool.Pool
}

func NewFlairRepo(pool *pgxpool.Pool) *FlairRepo {
	return &FlairRepo{pool: pool}
}

// ListCommunityFlairs returns all preset flairs for a community.
func (r *FlairRepo) ListCommunityFlairs(ctx context.Context, communityID string) ([]CommunityFlair, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, community_id, label, color, mod_only, sort_order
		FROM community_flairs
		WHERE community_id = $1
		ORDER BY sort_order ASC, label ASC`, communityID)
	if err != nil {
		return nil, fmt.Errorf("list community flairs: %w", err)
	}
	defer rows.Close()

	var flairs []CommunityFlair
	for rows.Next() {
		var f CommunityFlair
		if err := rows.Scan(&f.ID, &f.CommunityID, &f.Label, &f.Color, &f.ModOnly, &f.SortOrder); err != nil {
			return nil, err
		}
		flairs = append(flairs, f)
	}
	return flairs, rows.Err()
}

// CreateFlair creates a new preset flair (mod action).
func (r *FlairRepo) CreateFlair(ctx context.Context, communityID, label, color string, modOnly bool) (*CommunityFlair, error) {
	if color == "" {
		color = "gray"
	}
	var f CommunityFlair
	err := r.pool.QueryRow(ctx, `
		INSERT INTO community_flairs (community_id, label, color, mod_only)
		VALUES ($1, $2, $3, $4)
		RETURNING id, community_id, label, color, mod_only, sort_order`,
		communityID, label, color, modOnly,
	).Scan(&f.ID, &f.CommunityID, &f.Label, &f.Color, &f.ModOnly, &f.SortOrder)
	if err != nil {
		return nil, fmt.Errorf("create flair: %w", err)
	}
	return &f, nil
}

// DeleteFlair deletes a preset flair and cascades to remove any assignments.
func (r *FlairRepo) DeleteFlair(ctx context.Context, flairID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM community_flairs WHERE id = $1`, flairID)
	return err
}

// AssignFlair sets a participant's flair in a community (upsert — one flair per community per user).
func (r *FlairRepo) AssignFlair(ctx context.Context, participantID, communityID, flairID string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO participant_flairs (participant_id, community_id, flair_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (participant_id, community_id)
		DO UPDATE SET flair_id = EXCLUDED.flair_id, assigned_at = NOW()`,
		participantID, communityID, flairID)
	return err
}

// RemoveFlair clears a participant's flair in a community.
func (r *FlairRepo) RemoveFlair(ctx context.Context, participantID, communityID string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM participant_flairs
		WHERE participant_id = $1 AND community_id = $2`,
		participantID, communityID)
	return err
}

// GetFlair returns a participant's current flair in a community (joined with preset).
// Returns nil if no flair is assigned.
func (r *FlairRepo) GetFlair(ctx context.Context, participantID, communityID string) (*ParticipantFlair, error) {
	var f ParticipantFlair
	err := r.pool.QueryRow(ctx, `
		SELECT pf.participant_id, pf.community_id, pf.flair_id, cf.label, cf.color
		FROM participant_flairs pf
		JOIN community_flairs cf ON cf.id = pf.flair_id
		WHERE pf.participant_id = $1 AND pf.community_id = $2`,
		participantID, communityID).Scan(&f.ParticipantID, &f.CommunityID, &f.FlairID, &f.Label, &f.Color)
	if err != nil {
		return nil, err
	}
	return &f, nil
}
