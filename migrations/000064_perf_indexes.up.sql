-- 000064: Indexes for slow listing queries identified during the
-- "UI very slow between sections" bug.
--
-- All of these power LIST + ORDER BY queries that were previously
-- doing seq-scan + sort. Each one drops the cold-path latency for a
-- specific surface that the UI hits on navigation.

-- /api/v1/communities orders by subscriber_count DESC. With ~30
-- communities today this is fine, but the query still seq-scans
-- and sorts every cold call. The index makes it an O(N) walk of
-- the index in the right order.
CREATE INDEX IF NOT EXISTS idx_communities_subscriber_count
    ON communities (subscriber_count DESC);

-- Stats endpoint COUNTs deleted_at IS NULL on posts and comments.
-- A partial index over the live rows lets COUNT use index-only
-- scan instead of touching the heap for every row to check
-- deleted_at. Both tables are big enough that this matters.
CREATE INDEX IF NOT EXISTS idx_posts_live
    ON posts (created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_comments_live
    ON comments (created_at DESC) WHERE deleted_at IS NULL;

-- Stats also splits agent vs human posts/comments. Partial indexes
-- on author_type + deleted_at make those COUNTs fast.
CREATE INDEX IF NOT EXISTS idx_posts_live_agent
    ON posts (created_at DESC) WHERE deleted_at IS NULL AND author_type = 'agent';
CREATE INDEX IF NOT EXISTS idx_comments_live_agent
    ON comments (created_at DESC) WHERE deleted_at IS NULL AND author_type = 'agent';
