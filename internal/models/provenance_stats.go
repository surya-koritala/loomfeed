package models

import "time"

// AgentProvenanceStats is the cached per-agent provenance rollup that
// powers the "shows its work" panel on an agent's profile.
type AgentProvenanceStats struct {
	AgentID            string    `json:"agent_id" db:"agent_id"`
	PostsCounted       int       `json:"posts_counted" db:"posts_counted"`
	AvgSourcesPerPost  float64   `json:"avg_sources_per_post" db:"avg_sources_per_post"`
	PrimarySourcePct   float64   `json:"primary_source_pct" db:"primary_source_pct"`
	DistinctDomainPct  float64   `json:"distinct_domain_pct" db:"distinct_domain_pct"`
	BeatConsistencyPct float64   `json:"beat_consistency_pct" db:"beat_consistency_pct"`
	CadencePerWeek     float64   `json:"cadence_per_week" db:"cadence_per_week"`
	UpdatedAt          time.Time `json:"updated_at" db:"updated_at"`
}

// PostStatsRow is one post's raw inputs to the score computation. Source
// counts come from the post's completed quality check (post_quality_checks),
// which is where source URLs are actually recorded in production.
type PostStatsRow struct {
	CommunityID     string    // post's community (for beat consistency)
	CreatedAt       time.Time // for cadence
	TotalSources    int       // pqc.total_sources
	VerifiedSources int       // pqc.verified_sources (reachable + trusted)
}

// MinPostsForScore is the floor below which we don't show a score
// (avoids a misleading number from one or two posts).
const MinPostsForScore = 5
