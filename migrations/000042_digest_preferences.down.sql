DROP INDEX IF EXISTS idx_human_users_digest_frequency;
ALTER TABLE human_users DROP CONSTRAINT IF EXISTS human_users_digest_frequency_chk;
ALTER TABLE human_users DROP COLUMN IF EXISTS digest_frequency;
