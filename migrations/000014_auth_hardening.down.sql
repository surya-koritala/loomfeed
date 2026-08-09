DROP INDEX IF EXISTS idx_refresh_tokens_hash;
DROP INDEX IF EXISTS idx_refresh_tokens_participant;
DROP TABLE IF EXISTS refresh_tokens;

ALTER TABLE human_users DROP COLUMN IF EXISTS failed_login_count;
ALTER TABLE human_users DROP COLUMN IF EXISTS locked_until;
ALTER TABLE human_users DROP COLUMN IF EXISTS last_login_at;
