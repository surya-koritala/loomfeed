package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/surya-koritala/loomfeed/internal/models"
)

// QualityGateRepo owns the per-community quality_gates policy row.
type QualityGateRepo struct {
	pool *pgxpool.Pool
}

func NewQualityGateRepo(pool *pgxpool.Pool) *QualityGateRepo {
	return &QualityGateRepo{pool: pool}
}

// GetByCommunityID returns the configured gate. A community without a row has
// no additional gate; callers should handle pgx.ErrNoRows as the disabled state.
func (r *QualityGateRepo) GetByCommunityID(ctx context.Context, communityID string) (*models.QualityGate, error) {
	var gate models.QualityGate
	err := r.pool.QueryRow(ctx, `
		SELECT id, community_id, min_trust_score, min_confidence_score,
		       require_provenance, require_human_verification,
		       max_agent_posts_per_hour
		FROM quality_gates
		WHERE community_id = $1`, communityID,
	).Scan(
		&gate.ID,
		&gate.CommunityID,
		&gate.MinTrustScore,
		&gate.MinConfidenceScore,
		&gate.RequireProvenance,
		&gate.RequireHumanVerify,
		&gate.MaxAgentPostsPerHour,
	)
	if err != nil {
		return nil, err
	}
	return &gate, nil
}

// Upsert replaces the complete policy for a community.
func (r *QualityGateRepo) Upsert(ctx context.Context, gate *models.QualityGate) (*models.QualityGate, error) {
	var result models.QualityGate
	err := r.pool.QueryRow(ctx, `
		INSERT INTO quality_gates
		  (community_id, min_trust_score, min_confidence_score,
		   require_provenance, require_human_verification,
		   max_agent_posts_per_hour)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (community_id) DO UPDATE SET
		  min_trust_score = EXCLUDED.min_trust_score,
		  min_confidence_score = EXCLUDED.min_confidence_score,
		  require_provenance = EXCLUDED.require_provenance,
		  require_human_verification = EXCLUDED.require_human_verification,
		  max_agent_posts_per_hour = EXCLUDED.max_agent_posts_per_hour
		RETURNING id, community_id, min_trust_score, min_confidence_score,
		          require_provenance, require_human_verification,
		          max_agent_posts_per_hour`,
		gate.CommunityID,
		gate.MinTrustScore,
		gate.MinConfidenceScore,
		gate.RequireProvenance,
		gate.RequireHumanVerify,
		gate.MaxAgentPostsPerHour,
	).Scan(
		&result.ID,
		&result.CommunityID,
		&result.MinTrustScore,
		&result.MinConfidenceScore,
		&result.RequireProvenance,
		&result.RequireHumanVerify,
		&result.MaxAgentPostsPerHour,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert quality gate: %w", err)
	}
	return &result, nil
}

// CountRecentAgentPosts counts this agent's live posts in the community over
// the rolling prior hour. The caller compares it to MaxAgentPostsPerHour.
func (r *QualityGateRepo) CountRecentAgentPosts(ctx context.Context, communityID, authorID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM posts
		WHERE community_id = $1
		  AND author_id = $2
		  AND author_type = 'agent'
		  AND deleted_at IS NULL
		  AND created_at > NOW() - INTERVAL '1 hour'`,
		communityID,
		authorID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count recent agent posts: %w", err)
	}
	return count, nil
}
