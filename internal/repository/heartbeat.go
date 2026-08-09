package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HeartbeatRepo handles agent online/offline tracking via heartbeat pings.
type HeartbeatRepo struct {
	pool *pgxpool.Pool
}

// NewHeartbeatRepo creates a new HeartbeatRepo.
func NewHeartbeatRepo(pool *pgxpool.Pool) *HeartbeatRepo {
	return &HeartbeatRepo{pool: pool}
}

// Ping records a heartbeat and marks agent as online.
func (r *HeartbeatRepo) Ping(ctx context.Context, agentID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE agent_identities SET last_heartbeat_at = NOW(), is_online = TRUE, last_seen_at = NOW()
		 WHERE participant_id = $1`, agentID)
	return err
}

// MarkOffline marks agents as offline if no heartbeat in the last timeout period.
// Returns the number of agents marked offline.
func (r *HeartbeatRepo) MarkOffline(ctx context.Context, timeout time.Duration) (int, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE agent_identities SET is_online = FALSE
		 WHERE is_online = TRUE AND (last_heartbeat_at IS NULL OR last_heartbeat_at < NOW() - $1::interval)`,
		fmt.Sprintf("%d seconds", int(timeout.Seconds())))
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// OnlineAgentEntry represents a currently online agent's public info.
type OnlineAgentEntry struct {
	ID              string     `json:"id"`
	DisplayName     string     `json:"display_name"`
	AvatarURL       string     `json:"avatar_url,omitempty"`
	TrustScore      float64    `json:"trust_score"`
	ModelProvider   string     `json:"model_provider"`
	ModelName       string     `json:"model_name"`
	LastHeartbeatAt *time.Time `json:"last_heartbeat_at"`
	IsOnline        bool       `json:"is_online"`
}

// ListOnline returns currently online agents, ordered by most recently seen.
func (r *HeartbeatRepo) ListOnline(ctx context.Context, limit int) ([]OnlineAgentEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.id, p.display_name,
		       COALESCE(p.avatar_url, '') as avatar_url,
		       p.trust_score, ai.model_provider, ai.model_name,
		       ai.last_heartbeat_at, ai.is_online
		FROM participants p
		JOIN agent_identities ai ON ai.participant_id = p.id
		WHERE ai.is_online = TRUE
		ORDER BY ai.last_heartbeat_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []OnlineAgentEntry
	for rows.Next() {
		var a OnlineAgentEntry
		if err := rows.Scan(
			&a.ID, &a.DisplayName, &a.AvatarURL,
			&a.TrustScore, &a.ModelProvider, &a.ModelName,
			&a.LastHeartbeatAt, &a.IsOnline,
		); err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	if agents == nil {
		agents = []OnlineAgentEntry{}
	}
	return agents, rows.Err()
}

// OnlineCount returns the count of currently online agents.
func (r *HeartbeatRepo) OnlineCount(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM agent_identities WHERE is_online = TRUE`).Scan(&count)
	return count, err
}

// Pool exposes the underlying pgxpool for direct queries (e.g. activity logging).
func (r *HeartbeatRepo) Pool() *pgxpool.Pool {
	return r.pool
}
