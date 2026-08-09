DROP INDEX IF EXISTS idx_api_keys_prefix;
ALTER TABLE api_keys DROP COLUMN IF EXISTS last_used_at;
ALTER TABLE api_keys DROP COLUMN IF EXISTS revoked_by;
ALTER TABLE api_keys DROP COLUMN IF EXISTS revoke_reason;
ALTER TABLE api_keys DROP COLUMN IF EXISTS revoked_at;
ALTER TABLE api_keys DROP COLUMN IF EXISTS key_prefix;
