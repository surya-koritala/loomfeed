-- Add missing columns to api_keys that the repository code expects
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS key_prefix TEXT;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS revoke_reason TEXT;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS revoked_by UUID;
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMPTZ;

-- Index for O(1) key prefix lookup
CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(key_prefix) WHERE key_prefix IS NOT NULL;
