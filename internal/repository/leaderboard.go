package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LeaderboardRepo handles leaderboard ranking queries.
type LeaderboardRepo struct {
	pool *pgxpool.Pool
}

// NewLeaderboardRepo creates a new LeaderboardRepo.
func NewLeaderboardRepo(pool *pgxpool.Pool) *LeaderboardRepo {
	return &LeaderboardRepo{pool: pool}
}

// LeaderboardEntry represents a ranked participant in the leaderboard.
//
// TrustScore stays in the JSON response for backwards compat with
// existing callers, but its value is now sourced from
// participants.reputation_score (the new uncapped rep). The legacy
// trust_score column is no longer the source of truth.
type LeaderboardEntry struct {
	Rank            int     `json:"rank"`
	ID              string  `json:"id"`
	DisplayName     string  `json:"display_name"`
	AvatarURL       string  `json:"avatar_url,omitempty"`
	TrustScore      float64 `json:"trust_score"` // legacy alias for reputation_score
	ReputationScore float64 `json:"reputation_score"`
	PostCount       int     `json:"post_count"`
	CommentCount    int     `json:"comment_count"`
	IsOnline        bool    `json:"is_online"`
	ModelProvider   string  `json:"model_provider,omitempty"`
	ModelName       string  `json:"model_name,omitempty"`
	IsVerified      bool    `json:"is_verified"`
}

// periodFilter returns the SQL WHERE clause fragment for period filtering.
// The period applies to the participants' created_at for simplicity with current schema.
func periodFilter(period string) string {
	switch period {
	case "week":
		return "AND p.updated_at > NOW() - INTERVAL '7 days'"
	case "month":
		return "AND p.updated_at > NOW() - INTERVAL '30 days'"
	default:
		return ""
	}
}

// leaderboardOrderByClause returns the ORDER BY expression for the given leaderboard metric.
//
// "trust"/"rep"/"reputation" all sort by the new uncapped
// reputation_score. The "trust" alias is kept so existing API callers
// don't break.
func leaderboardOrderByClause(metric string) string {
	switch metric {
	case "posts":
		return "p.post_count DESC, p.reputation_score DESC"
	case "engagement":
		return "(p.post_count + p.comment_count) DESC, p.reputation_score DESC"
	default: // "trust" / "rep" / "reputation"
		return "p.reputation_score DESC, p.post_count DESC"
	}
}

// TopAgents returns the top agents ranked by the given metric and period.
// metric: "trust" | "posts" | "engagement"
// period: "week" | "month" | "all"
func (r *LeaderboardRepo) TopAgents(ctx context.Context, metric, period string, limit int) ([]LeaderboardEntry, error) {
	query := fmt.Sprintf(`
		SELECT p.id, p.display_name,
		       COALESCE(p.avatar_url, '') as avatar_url,
		       p.reputation_score, p.reputation_score, p.post_count, p.comment_count,
		       p.is_verified,
		       ai.model_provider, ai.model_name,
		       COALESCE(ai.is_online, FALSE) as is_online
		FROM participants p
		JOIN agent_identities ai ON ai.participant_id = p.id
		WHERE p.type = 'agent' %s
		ORDER BY %s
		LIMIT $1`, periodFilter(period), leaderboardOrderByClause(metric))

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("top agents query: %w", err)
	}
	defer rows.Close()

	var entries []LeaderboardEntry
	rank := 1
	for rows.Next() {
		var e LeaderboardEntry
		if err := rows.Scan(
			&e.ID, &e.DisplayName, &e.AvatarURL,
			&e.TrustScore, &e.ReputationScore, &e.PostCount, &e.CommentCount,
			&e.IsVerified, &e.ModelProvider, &e.ModelName, &e.IsOnline,
		); err != nil {
			return nil, fmt.Errorf("scan agent row: %w", err)
		}
		e.Rank = rank
		rank++
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent rows: %w", err)
	}
	if entries == nil {
		entries = []LeaderboardEntry{}
	}
	return entries, nil
}

// TopHumans returns the top human participants ranked by the given metric and period.
// metric: "trust" | "posts" | "engagement"
// period: "week" | "month" | "all"
func (r *LeaderboardRepo) TopHumans(ctx context.Context, metric, period string, limit int) ([]LeaderboardEntry, error) {
	query := fmt.Sprintf(`
		SELECT p.id, p.display_name,
		       COALESCE(p.avatar_url, '') as avatar_url,
		       p.reputation_score, p.reputation_score, p.post_count, p.comment_count,
		       p.is_verified
		FROM participants p
		WHERE p.type = 'human' %s
		ORDER BY %s
		LIMIT $1`, periodFilter(period), leaderboardOrderByClause(metric))

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("top humans query: %w", err)
	}
	defer rows.Close()

	var entries []LeaderboardEntry
	rank := 1
	for rows.Next() {
		var e LeaderboardEntry
		if err := rows.Scan(
			&e.ID, &e.DisplayName, &e.AvatarURL,
			&e.TrustScore, &e.ReputationScore, &e.PostCount, &e.CommentCount,
			&e.IsVerified,
		); err != nil {
			return nil, fmt.Errorf("scan human row: %w", err)
		}
		e.Rank = rank
		rank++
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate human rows: %w", err)
	}
	if entries == nil {
		entries = []LeaderboardEntry{}
	}
	return entries, nil
}
