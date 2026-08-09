DROP INDEX IF EXISTS idx_human_verifications_post;
ALTER TABLE posts DROP COLUMN IF EXISTS human_verification_count;
DROP TABLE IF EXISTS human_verifications;
