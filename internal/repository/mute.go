package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MuteRepo handles community_mutes.
type MuteRepo struct {
	pool *pgxpool.Pool
}

// NewMuteRepo creates a MuteRepo.
func NewMuteRepo(pool *pgxpool.Pool) *MuteRepo {
	return &MuteRepo{pool: pool}
}

// Mute inserts (participant, community). Idempotent.
func (r *MuteRepo) Mute(ctx context.Context, participantID, communityID string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO community_mutes (participant_id, community_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`,
		participantID, communityID)
	if err != nil {
		return fmt.Errorf("insert mute: %w", err)
	}
	return nil
}

// Unmute removes (participant, community). No-op if no row exists.
func (r *MuteRepo) Unmute(ctx context.Context, participantID, communityID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM community_mutes WHERE participant_id = $1 AND community_id = $2`,
		participantID, communityID)
	if err != nil {
		return fmt.Errorf("delete mute: %w", err)
	}
	return nil
}

// IsMuted reports whether participant has muted community.
func (r *MuteRepo) IsMuted(ctx context.Context, participantID, communityID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM community_mutes
		   WHERE participant_id = $1 AND community_id = $2)`,
		participantID, communityID).Scan(&exists)
	return exists, err
}

// ListMutedIDs returns the community IDs muted by the participant.
func (r *MuteRepo) ListMutedIDs(ctx context.Context, participantID string) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT community_id FROM community_mutes WHERE participant_id = $1`,
		participantID)
	if err != nil {
		return nil, fmt.Errorf("list muted ids: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan muted id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// MutedRow is the rendering-ready shape returned by
// ListMutedWithDetails for /settings.
type MutedRow struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	MutedAt     time.Time `json:"muted_at"`
}

// ListMutedWithDetails returns every community muted by the user
// joined with the community's identifying fields.
func (r *MuteRepo) ListMutedWithDetails(ctx context.Context, participantID string) ([]MutedRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
		  c.id,
		  c.slug,
		  c.name,
		  COALESCE(c.description, ''),
		  m.created_at
		FROM community_mutes m
		JOIN communities c ON c.id = m.community_id
		WHERE m.participant_id = $1
		ORDER BY m.created_at DESC`,
		participantID)
	if err != nil {
		return nil, fmt.Errorf("list muted with details: %w", err)
	}
	defer rows.Close()
	out := make([]MutedRow, 0)
	for rows.Next() {
		var m MutedRow
		if err := rows.Scan(&m.ID, &m.Slug, &m.Name, &m.Description, &m.MutedAt); err != nil {
			return nil, fmt.Errorf("scan muted row: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
