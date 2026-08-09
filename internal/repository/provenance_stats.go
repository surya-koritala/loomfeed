package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/RoamXAI/loomfeed/internal/models"
)

type ProvenanceStatsRepo struct {
	pool *pgxpool.Pool
}

func NewProvenanceStatsRepo(pool *pgxpool.Pool) *ProvenanceStatsRepo {
	return &ProvenanceStatsRepo{pool: pool}
}

// FetchAgentPostsForStats returns one PostStatsRow per non-deleted post by the
// agent created after `since`. Per-post source counts come from the post's
// COMPLETED quality check (post_quality_checks) — the same data behind the
// "N sources · M verified" badge and the canonical place source URLs are
// recorded in production. The earlier provenances.sources column is unpopulated
// in prod, which is why the score read all-zeros. Mirrors the feed query's
// join: `pqc.post_id = p.id AND pqc.status = 'complete'`.
func (r *ProvenanceStatsRepo) FetchAgentPostsForStats(ctx context.Context, agentID string, since time.Time) ([]models.PostStatsRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.community_id, p.created_at,
		       COALESCE(pqc.total_sources, 0)    AS total_sources,
		       COALESCE(pqc.verified_sources, 0) AS verified_sources
		FROM posts p
		LEFT JOIN post_quality_checks pqc
		       ON pqc.post_id = p.id AND pqc.status = 'complete'
		WHERE p.author_id = $1
		  AND p.deleted_at IS NULL
		  AND p.created_at >= $2`,
		agentID, since)
	if err != nil {
		return nil, fmt.Errorf("fetch agent posts for stats: %w", err)
	}
	defer rows.Close()

	var out []models.PostStatsRow
	for rows.Next() {
		var row models.PostStatsRow
		if err := rows.Scan(&row.CommunityID, &row.CreatedAt, &row.TotalSources, &row.VerifiedSources); err != nil {
			return nil, fmt.Errorf("scan post stats row: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// Upsert writes the computed stats for one agent.
func (r *ProvenanceStatsRepo) Upsert(ctx context.Context, s models.AgentProvenanceStats) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO agent_provenance_stats
		  (agent_id, posts_counted, avg_sources_per_post, primary_source_pct,
		   distinct_domain_pct, beat_consistency_pct, cadence_per_week, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7, NOW())
		ON CONFLICT (agent_id) DO UPDATE SET
		  posts_counted        = EXCLUDED.posts_counted,
		  avg_sources_per_post = EXCLUDED.avg_sources_per_post,
		  primary_source_pct   = EXCLUDED.primary_source_pct,
		  distinct_domain_pct  = EXCLUDED.distinct_domain_pct,
		  beat_consistency_pct = EXCLUDED.beat_consistency_pct,
		  cadence_per_week     = EXCLUDED.cadence_per_week,
		  updated_at           = NOW()`,
		s.AgentID, s.PostsCounted, s.AvgSourcesPerPost, s.PrimarySourcePct,
		s.DistinctDomainPct, s.BeatConsistencyPct, s.CadencePerWeek)
	if err != nil {
		return fmt.Errorf("upsert agent provenance stats: %w", err)
	}
	return nil
}

// Get returns the stats for an agent, or (nil, nil) if none exist.
func (r *ProvenanceStatsRepo) Get(ctx context.Context, agentID string) (*models.AgentProvenanceStats, error) {
	var s models.AgentProvenanceStats
	err := r.pool.QueryRow(ctx, `
		SELECT agent_id, posts_counted, avg_sources_per_post, primary_source_pct,
		       distinct_domain_pct, beat_consistency_pct, cadence_per_week, updated_at
		FROM agent_provenance_stats WHERE agent_id = $1`, agentID).
		Scan(&s.AgentID, &s.PostsCounted, &s.AvgSourcesPerPost, &s.PrimarySourcePct,
			&s.DistinctDomainPct, &s.BeatConsistencyPct, &s.CadencePerWeek, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get agent provenance stats: %w", err)
	}
	return &s, nil
}

// AllAgentIDs returns every agent participant id (for the nightly sweep).
func (r *ProvenanceStatsRepo) AllAgentIDs(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT id FROM participants WHERE type = 'agent'`)
	if err != nil {
		return nil, fmt.Errorf("all agent ids: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan agent id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
