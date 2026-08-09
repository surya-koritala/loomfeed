-- Phase: Looms — table + Loom participant + comment annotations.
--
-- The companion enum value 'loom' is added in 000078; this migration
-- depends on that having committed.

-- The Loom participant. Fixed UUID so code can reference it as a
-- constant. ON CONFLICT keeps the migration safe to re-run against an
-- already-seeded DB (e.g. a dev box that was hand-poked).
INSERT INTO participants (id, type, display_name, bio, is_verified)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'loom',
    'Loom',
    'The platform-operated AI. Summon with @loom. Can make mistakes — always verify.',
    TRUE
)
ON CONFLICT (id) DO NOTHING;

-- Audit log of every @loom invocation. Drives cost telemetry, rate
-- limiting, debugging, and the eventual eval pipeline. One row per
-- summon, regardless of cache outcome.
CREATE TABLE loom_summons (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    participant_id   UUID REFERENCES participants(id) ON DELETE SET NULL,
    post_id          UUID REFERENCES posts(id) ON DELETE CASCADE,
    comment_id       UUID REFERENCES comments(id) ON DELETE CASCADE,
    reply_comment_id UUID REFERENCES comments(id) ON DELETE SET NULL,
    intent           TEXT NOT NULL,
    prompt           TEXT NOT NULL,
    response         TEXT,
    model            TEXT,
    input_tokens     INT,
    output_tokens    INT,
    cost_usd         NUMERIC(10, 6),
    cached           BOOLEAN NOT NULL DEFAULT FALSE,
    state            TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'done', 'error')),
    error_code       TEXT,
    latency_ms       INT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at      TIMESTAMPTZ
);

-- Daily rate-limit scan walks (participant_id, created_at) — count
-- rows where created_at > now() - 24h. Index supports both that scan
-- and the per-user history view.
CREATE INDEX idx_loom_summons_participant_day
    ON loom_summons(participant_id, created_at DESC);

-- The worker picks up pending summons here. Partial index keeps it
-- O(pending) — done/error rows don't bloat the scan.
CREATE INDEX idx_loom_summons_pending
    ON loom_summons(created_at)
    WHERE state = 'pending';

-- Loom replies are regular comments authored by the Loom participant,
-- so threading / voting / reactions / federation work for free. The
-- two new columns let the UI render the Loom badge + specialty tag
-- without joining loom_summons just to detect that a comment is from
-- the AI.
ALTER TABLE comments
    ADD COLUMN loom_summon_id UUID REFERENCES loom_summons(id) ON DELETE SET NULL,
    ADD COLUMN loom_intent    TEXT;

-- Fast "show me Loom replies in this thread" query for analytics.
CREATE INDEX idx_comments_loom
    ON comments(post_id, loom_summon_id)
    WHERE loom_summon_id IS NOT NULL;
