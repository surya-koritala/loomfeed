-- Reactions beyond up/down.
--
-- Two changes:
--   1. Extend the reaction_type enum with the four Loomfeed-native
--      semantic reactions: confirmed (I've verified this),
--      contradicts (this contradicts what I know), cites_this (this
--      cites a source I find credible). 'insightful' and
--      'needs_citation' already existed.
--   2. Let reactions target posts as well as comments. comment_id and
--      post_id are both nullable; a CHECK constraint enforces that
--      exactly one is set so we can tell what a reaction is on
--      without a separate type column.

-- Enum extensions — IF NOT EXISTS so re-running is safe.
ALTER TYPE reaction_type ADD VALUE IF NOT EXISTS 'confirmed';
ALTER TYPE reaction_type ADD VALUE IF NOT EXISTS 'contradicts';
ALTER TYPE reaction_type ADD VALUE IF NOT EXISTS 'cites_this';

-- Add post_id and loosen comment_id. Using a 2-statement dance so the
-- existing rows (all comment-scoped) don't violate the new NULL rule.
ALTER TABLE reactions ADD COLUMN IF NOT EXISTS post_id UUID
    REFERENCES posts(id) ON DELETE CASCADE;
ALTER TABLE reactions ALTER COLUMN comment_id DROP NOT NULL;

-- Exactly one target must be set, never both, never neither.
ALTER TABLE reactions DROP CONSTRAINT IF EXISTS reactions_target_chk;
ALTER TABLE reactions ADD CONSTRAINT reactions_target_chk
    CHECK ((comment_id IS NULL) <> (post_id IS NULL));

-- The original UNIQUE (comment_id, participant_id, reaction_type)
-- guarded against double-reacting on a comment. We need a parallel
-- one for posts. Partial unique indexes, one per target kind.
DROP INDEX IF EXISTS idx_reactions_unique_per_comment;
CREATE UNIQUE INDEX IF NOT EXISTS idx_reactions_unique_per_comment
    ON reactions(comment_id, participant_id, reaction_type)
    WHERE comment_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_reactions_unique_per_post
    ON reactions(post_id, participant_id, reaction_type)
    WHERE post_id IS NOT NULL;

-- The old UNIQUE (comment_id, participant_id, reaction_type) shape
-- would start rejecting NULL comment_ids as identical rows in some
-- Postgres versions; drop it in favor of the partial index above.
ALTER TABLE reactions DROP CONSTRAINT IF EXISTS reactions_comment_id_participant_id_reaction_type_key;

-- Fast "reactions on this post" lookup for the new post endpoint.
CREATE INDEX IF NOT EXISTS idx_reactions_post
    ON reactions(post_id)
    WHERE post_id IS NOT NULL;
