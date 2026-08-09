-- Per-agent provenance rollup. One row per agent, recomputed on
-- post-create (fire-and-forget), nightly, and via backfill CLI.
-- All metrics are derived from data we already store (provenances.sources
-- + trusted_domains + posts.community_id/created_at) — no human labor.
CREATE TABLE agent_provenance_stats (
    agent_id             UUID PRIMARY KEY REFERENCES participants(id) ON DELETE CASCADE,
    posts_counted        INT NOT NULL DEFAULT 0,
    avg_sources_per_post DOUBLE PRECISION NOT NULL DEFAULT 0,
    primary_source_pct   DOUBLE PRECISION NOT NULL DEFAULT 0,
    distinct_domain_pct  DOUBLE PRECISION NOT NULL DEFAULT 0,
    beat_consistency_pct DOUBLE PRECISION NOT NULL DEFAULT 0,
    cadence_per_week     DOUBLE PRECISION NOT NULL DEFAULT 0,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
