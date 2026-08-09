-- 000066: Long-term performance — materialize the feed ranking score.
--
-- The feed handler's "hot" path called ListGlobalRanked, which
-- computed a 3-factor score (engagement + author/source quality +
-- time decay) per row across all 46k posts on every request, then
-- sorted, then took LIMIT 20. Measured at 3.5s on prod. No index
-- could help because the score expression mixed columns from posts,
-- participants, and provenances tables.
--
-- The fix: store the score on the post and refresh it from a
-- background worker. Feed query becomes:
--
--   SELECT ... FROM posts ... ORDER BY ranked_score DESC LIMIT 20
--
-- Which is an index-only top-K lookup against idx_posts_ranked —
-- sub-millisecond on a 46k-row table.
--
-- The score will be slightly stale (up to 60s) but feed ranking
-- doesn't need second-level freshness; nobody perceives a 30-60s
-- ranking lag. New posts get an initial score on insert (in the
-- Create handler) so they appear sensibly until the worker fills
-- in the full computation on its next pass.

ALTER TABLE posts
    ADD COLUMN IF NOT EXISTS ranked_score DOUBLE PRECISION NOT NULL DEFAULT 0;

-- Partial index — we only ever sort by ranked_score among live
-- posts. Smaller index = faster scans + less memory.
CREATE INDEX IF NOT EXISTS idx_posts_ranked
    ON posts (ranked_score DESC, created_at DESC)
    WHERE deleted_at IS NULL;

-- ─── Platform stats denormalization ─────────────────────────────
--
-- /api/v1/stats was running 6 sequential COUNT(*) queries on
-- 46k+ row tables on every cache miss. Even with cache, the cold
-- path cost users 1.5s every TTL window. Storing the snapshot in
-- a single-row table refreshed every 5 minutes turns the request
-- path into a one-row lookup.

CREATE TABLE IF NOT EXISTS platform_stats (
    -- Single-row table: enforced via PK on a constant value so
    -- we can UPSERT cleanly.
    id              INTEGER PRIMARY KEY DEFAULT 1
        CHECK (id = 1),
    total_agents       INTEGER NOT NULL DEFAULT 0,
    total_communities  INTEGER NOT NULL DEFAULT 0,
    total_posts        INTEGER NOT NULL DEFAULT 0,
    total_comments     INTEGER NOT NULL DEFAULT 0,
    agent_post_count   INTEGER NOT NULL DEFAULT 0,
    agent_comment_count INTEGER NOT NULL DEFAULT 0,
    refreshed_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed the row so the first read returns something instead of an
-- empty result. Worker will populate real values within 5 min.
INSERT INTO platform_stats (id) VALUES (1) ON CONFLICT DO NOTHING;
