-- Phase 0.4 — new-account spam quarantine.
--
-- Posts from accounts that are <48h old AND under trust threshold
-- are held back from the public feed until either (a) a moderator
-- approves them, or (b) the platform's automated graduation rule
-- fires when the user crosses thresholds organically. Without
-- this layer, signup floodgates open the whole platform to bots
-- the moment they get past CAPTCHA.

-- Per-post boolean. Cheaper than a separate quarantines table —
-- the quarantine state belongs to the post (not the author) since
-- a single user might have a mix of pre- and post-graduation
-- posts during the transition. Also: a moderator approving one
-- post doesn't necessarily graduate the account, hence
-- post-level + account-level fields side-by-side.
ALTER TABLE posts
    ADD COLUMN IF NOT EXISTS quarantined BOOLEAN NOT NULL DEFAULT FALSE;

-- Partial index — most rows are FALSE so a full index is wasteful.
-- The mod queue ("show me posts pending review in my community")
-- and feed ("hide quarantined from public") both filter to
-- quarantined=TRUE.
CREATE INDEX IF NOT EXISTS idx_posts_quarantined
    ON posts(community_id, created_at DESC)
    WHERE quarantined = TRUE;

-- Account-level "this user is past the new-account window" flag.
-- Any post by a graduated user skips the quarantine check entirely;
-- the value is the timestamp at which graduation happened, useful
-- for audit ("when did Alice get past the new-account gate?").
ALTER TABLE participants
    ADD COLUMN IF NOT EXISTS graduated_at TIMESTAMPTZ;

-- Backfill: every existing participant counts as already graduated
-- so the rollout of this feature doesn't suddenly hide every post
-- on the platform from new users. Use created_at as the
-- approximate graduation moment.
UPDATE participants
   SET graduated_at = created_at
 WHERE graduated_at IS NULL;
