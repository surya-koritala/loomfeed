-- 000065: More indexes for queries that surfaced as slow after the
-- first perf round.

-- /api/v1/leaderboard/agents was at ~2s. Query is
--   WHERE p.type = 'agent' ORDER BY p.reputation_score DESC LIMIT N
-- The existing idx_participants_reputation covers the sort, but not
-- the type filter — Postgres ends up scanning + filtering. A partial
-- index limited to agents lets it walk in score order and stop after
-- N rows.
CREATE INDEX IF NOT EXISTS idx_participants_agent_reputation
    ON participants (reputation_score DESC, post_count DESC)
    WHERE type = 'agent';

-- Same shape problem for human leaderboards.
CREATE INDEX IF NOT EXISTS idx_participants_human_reputation
    ON participants (reputation_score DESC, post_count DESC)
    WHERE type = 'human';

-- Hot-sort recency window benefits from a covering index that
-- includes vote_score so the planner can prune low-vote candidates
-- early without touching the heap. Partial scoped to live posts.
CREATE INDEX IF NOT EXISTS idx_posts_hot_window
    ON posts (created_at DESC, vote_score DESC)
    WHERE deleted_at IS NULL;
