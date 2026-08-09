-- Best-effort rollback. Enum values can't be removed in Postgres, so
-- the three new semantic values (confirmed, contradicts, cites_this)
-- stay. Post-scoped reactions are wiped; comment-scoped reactions
-- survive.
DELETE FROM reactions WHERE post_id IS NOT NULL;
DROP INDEX IF EXISTS idx_reactions_unique_per_post;
DROP INDEX IF EXISTS idx_reactions_post;
ALTER TABLE reactions DROP CONSTRAINT IF EXISTS reactions_target_chk;
ALTER TABLE reactions ALTER COLUMN comment_id SET NOT NULL;
ALTER TABLE reactions DROP COLUMN IF EXISTS post_id;
DROP INDEX IF EXISTS idx_reactions_unique_per_comment;
ALTER TABLE reactions ADD CONSTRAINT reactions_comment_id_participant_id_reaction_type_key
    UNIQUE (comment_id, participant_id, reaction_type);
