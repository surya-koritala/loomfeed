package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ModerationRepo handles database operations for community moderation.
type ModerationRepo struct {
	pool *pgxpool.Pool
}

// NewModerationRepo creates a new ModerationRepo.
func NewModerationRepo(pool *pgxpool.Pool) *ModerationRepo {
	return &ModerationRepo{pool: pool}
}

// IsModerator returns true if participantID is a moderator of communityID.
func (r *ModerationRepo) IsModerator(ctx context.Context, communityID, participantID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM community_moderators WHERE community_id = $1 AND participant_id = $2)`,
		communityID, participantID).Scan(&exists)
	return exists, err
}

// AddModerator inserts a moderator record (ignores conflict if already exists).
func (r *ModerationRepo) AddModerator(ctx context.Context, communityID, participantID, role string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO community_moderators (community_id, participant_id, role) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		communityID, participantID, role)
	return err
}

// RemoveModerator deletes a moderator record.
func (r *ModerationRepo) RemoveModerator(ctx context.Context, communityID, participantID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM community_moderators WHERE community_id = $1 AND participant_id = $2`,
		communityID, participantID)
	return err
}

// ListModerators returns all moderators for a community joined with participant data.
func (r *ModerationRepo) ListModerators(ctx context.Context, communityID string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT p.id, p.display_name, p.type, p.trust_score, cm.role, cm.created_at
         FROM community_moderators cm
         JOIN participants p ON p.id = cm.participant_id
         WHERE cm.community_id = $1
         ORDER BY cm.created_at`, communityID)
	if err != nil {
		return nil, fmt.Errorf("list moderators: %w", err)
	}
	defer rows.Close()

	var mods []map[string]any
	for rows.Next() {
		var id, name, pType, role string
		var trust float64
		var createdAt interface{}
		if err := rows.Scan(&id, &name, &pType, &trust, &role, &createdAt); err != nil {
			return nil, fmt.Errorf("scan moderator row: %w", err)
		}
		mods = append(mods, map[string]any{
			"id": id, "display_name": name, "type": pType,
			"trust_score": trust, "role": role, "created_at": createdAt,
		})
	}
	return mods, rows.Err()
}

// GetPendingReports returns pending reports for posts in a community.
func (r *ModerationRepo) GetPendingReports(ctx context.Context, communityID string, limit int) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `
        SELECT r.id, r.reporter_id, r.content_id, r.content_type, r.reason,
               COALESCE(r.details, ''), r.status, r.created_at,
               rep.display_name as reporter_name
        FROM reports r
        JOIN participants rep ON rep.id = r.reporter_id
        JOIN posts p ON (r.content_type = 'post' AND r.content_id = p.id AND p.community_id = $1)
        WHERE r.status = 'pending'
        ORDER BY r.created_at DESC
        LIMIT $2`, communityID, limit)
	if err != nil {
		return nil, fmt.Errorf("get pending reports: %w", err)
	}
	defer rows.Close()

	var reports []map[string]any
	for rows.Next() {
		var id, reporterID, contentID, contentType, reason, details, status, reporterName string
		var createdAt interface{}
		if err := rows.Scan(&id, &reporterID, &contentID, &contentType, &reason, &details, &status, &createdAt, &reporterName); err != nil {
			return nil, fmt.Errorf("scan report row: %w", err)
		}
		reports = append(reports, map[string]any{
			"id": id, "reporter_id": reporterID, "reporter_name": reporterName,
			"content_id": contentID, "content_type": contentType,
			"reason": reason, "details": details, "status": status, "created_at": createdAt,
		})
	}
	return reports, rows.Err()
}
