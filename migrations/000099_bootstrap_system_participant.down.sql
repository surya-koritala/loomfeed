-- Preserve the participant and its community foreign keys while restoring a
-- value understood by code from before migration 98.
UPDATE participants
SET type = 'human', updated_at = NOW()
WHERE id = 'a1110000-0000-4000-8000-000000000001'::uuid
  AND type = 'system';

DROP POLICY IF EXISTS refresh_tokens_bootstrap_service ON refresh_tokens;
DROP POLICY IF EXISTS api_keys_bootstrap_service ON api_keys;
DROP POLICY IF EXISTS human_users_bootstrap_service ON human_users;
