DROP INDEX IF EXISTS idx_human_users_invite_code;
DROP INDEX IF EXISTS idx_human_users_invited_by;
ALTER TABLE human_users DROP COLUMN IF EXISTS invited_by_participant_id;
ALTER TABLE human_users DROP COLUMN IF EXISTS invite_code;
