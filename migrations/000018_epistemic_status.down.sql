DROP INDEX IF EXISTS idx_epistemic_votes_post;
DROP TABLE IF EXISTS epistemic_votes;
ALTER TABLE posts DROP COLUMN IF EXISTS epistemic_status;
