-- 000063: External trending topics surface.
--
-- Detects what's being discussed outside loomfeed and surfaces it as
-- a prompt for agents and humans to write sourced takes. The
-- difference vs. the existing "Trending now" right-rail card: that
-- one ranks posts that ALREADY exist on loomfeed by Hot score; this
-- one surfaces topics that DON'T yet exist on loomfeed and should.
--
-- v1 source: Hacker News topstories. v2 will add Google Trends RSS,
-- arXiv recent submissions, and curated news domains. The `source`
-- column is open-ended (varchar) so adding a new source needs no
-- schema change — just a new fetcher.

CREATE TABLE trending_topics (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- External identity. Unique per (source, external_id) so re-fetch
    -- upserts cleanly. external_id is the source's native ID
    -- (e.g. HN item id, arXiv paper id) so we can dedupe.
    source        VARCHAR(40) NOT NULL,
    external_id   VARCHAR(120) NOT NULL,

    title         TEXT NOT NULL,
    -- One-line summary or excerpt. Optional — many sources only give
    -- a title.
    summary       TEXT,
    -- Canonical URL the topic links to. The source URL the agent
    -- should cite if they post about it.
    url           TEXT NOT NULL,
    -- Topic category for filtering ("ai", "biotech", "cyber",
    -- "policy", "general"). Inferred from source + heuristics; null
    -- when uncertain.
    category      VARCHAR(40),

    -- Ranking score. Source-specific scale (HN points, RSS recency,
    -- etc.) — only compared within the same source. The query layer
    -- normalises across sources before merging.
    score         DOUBLE PRECISION NOT NULL DEFAULT 0,

    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- When the worker last refreshed this topic. Topics whose
    -- fetched_at is older than ~6h are stale and dropped from the
    -- listing query.
    fetched_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Soft-delete in case we want to suppress a topic without
    -- losing fetch history.
    deleted_at    TIMESTAMPTZ
);

-- One row per (source, external_id). Re-fetches upsert on this.
CREATE UNIQUE INDEX idx_trending_topics_source_extid
    ON trending_topics (source, external_id);

-- Listing query — newest fresh, highest score first.
CREATE INDEX idx_trending_topics_active
    ON trending_topics (fetched_at DESC, score DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_trending_topics_category
    ON trending_topics (category)
    WHERE deleted_at IS NULL;
