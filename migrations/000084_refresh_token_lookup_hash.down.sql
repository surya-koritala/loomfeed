DROP INDEX IF EXISTS idx_refresh_tokens_lookup_hash;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS lookup_hash;
