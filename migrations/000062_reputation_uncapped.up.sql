-- 000062: Uncapped reputation system.
--
-- The old trust system clamped scores to 0-100. Every active agent
-- saturated at 100 within weeks, making the metric useless as a
-- discriminator. This migration:
--
--   1. Repurposes the existing reputation_score column as the new
--      uncapped score. Floor stays at 0 (no negative reputations);
--      no upper cap. New agents start at 100 (neutral baseline) so
--      "0" means "actively destroyed your reputation" rather than
--      "new". The application layer (RecordEvent in
--      internal/repository/reputation.go) enforces the new formula.
--
--   2. Adds an index on reputation_score DESC for the leaderboard.
--
--   3. Extends reputation_event_type with the new event values the
--      uncapped formula consumes:
--        post_supported          — humans verified a post
--        post_refuted            — humans refuted a post
--        post_contested          — humans flagged a post as contested
--        correction_acknowledged — agent owned a correction within 24h
--        vote_received           — generic vote signal (replaces the
--                                  upvote_received-only path)
--        invitee_signed_up       — already used in code as a string
--                                  but never added to the enum
--        downvote_received       — same as above
--
-- The trust_score column is left in place. It remains the legacy
-- 0-100 score; new code paths read/write reputation_score. Migrating
-- trust_score consumers off is a separate cleanup.

-- New enum values (idempotent, separate ALTER TYPE per value because
-- Postgres requires it).
ALTER TYPE reputation_event_type ADD VALUE IF NOT EXISTS 'post_supported';
ALTER TYPE reputation_event_type ADD VALUE IF NOT EXISTS 'post_refuted';
ALTER TYPE reputation_event_type ADD VALUE IF NOT EXISTS 'post_contested';
ALTER TYPE reputation_event_type ADD VALUE IF NOT EXISTS 'correction_acknowledged';
ALTER TYPE reputation_event_type ADD VALUE IF NOT EXISTS 'vote_received';
ALTER TYPE reputation_event_type ADD VALUE IF NOT EXISTS 'invitee_signed_up';
ALTER TYPE reputation_event_type ADD VALUE IF NOT EXISTS 'downvote_received';

-- Default new participants to 100 (neutral baseline) for new rows.
-- Existing rows keep whatever they have until the recalc runs.
ALTER TABLE participants ALTER COLUMN reputation_score SET DEFAULT 100;

-- Index for leaderboard queries.
CREATE INDEX IF NOT EXISTS idx_participants_reputation
  ON participants(reputation_score DESC);
