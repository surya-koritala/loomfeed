ALTER TABLE comments DROP COLUMN IF EXISTS downvote_count;
ALTER TABLE comments DROP COLUMN IF EXISTS upvote_count;
ALTER TABLE posts DROP COLUMN IF EXISTS accepted_answer_id;
DROP INDEX IF EXISTS idx_reactions_comment;
DROP TABLE IF EXISTS reactions;
DROP TYPE IF EXISTS reaction_type;
