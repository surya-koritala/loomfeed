-- Curated shorts — external short-form videos (YouTube Shorts to
-- start; Bluesky / Mastodon video later) that an LLM + human
-- moderator vets for relevance, then serves on /shorts.
--
-- We never host video. Only metadata lives here; the iframe loads
-- the actual player straight from the source platform's CDN, which
-- keeps us inside every platform's ToS.
--
-- Status flow:
--   pending  -> LLM has queued it, waiting for human approval
--   approved -> visible on /shorts
--   rejected -> hidden (kept for training signal)
--   expired  -> creator deleted / platform yanked it (nightly check)

CREATE TABLE curated_shorts (
    id                      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    platform                VARCHAR(32)  NOT NULL,  -- 'youtube' | 'bluesky' | 'mastodon'
    platform_video_id       VARCHAR(128) NOT NULL,
    title                   TEXT NOT NULL,
    creator_name            TEXT NOT NULL DEFAULT '',
    creator_url             TEXT NOT NULL DEFAULT '',
    category                VARCHAR(32)  NOT NULL,  -- ai-research | robotics | science | ml-engineering | tech-critique
    embed_url               TEXT NOT NULL,
    watch_url               TEXT NOT NULL,
    thumbnail_url           TEXT NOT NULL DEFAULT '',
    duration_sec            INTEGER NOT NULL DEFAULT 0,
    view_count              BIGINT  NOT NULL DEFAULT 0,
    ai_score                DOUBLE PRECISION NOT NULL DEFAULT 0, -- 0..1 loomfeed-fit
    ai_rationale            TEXT NOT NULL DEFAULT '',
    status                  VARCHAR(16) NOT NULL DEFAULT 'pending',
    reviewed_by_id          UUID REFERENCES participants(id),
    reviewed_at             TIMESTAMPTZ,
    platform_published_at   TIMESTAMPTZ,
    curated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (platform, platform_video_id)
);

-- Serving path: approved shorts ordered newest first, filtered by
-- category. Partial index on status='approved' keeps it small.
CREATE INDEX idx_curated_shorts_feed
    ON curated_shorts (category, curated_at DESC)
    WHERE status = 'approved';

-- Admin path: pending queue ordered by LLM score desc so moderator
-- sees the best candidates first.
CREATE INDEX idx_curated_shorts_pending
    ON curated_shorts (ai_score DESC, curated_at DESC)
    WHERE status = 'pending';
