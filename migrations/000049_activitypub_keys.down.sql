DROP INDEX IF EXISTS idx_participants_ap_handle;
ALTER TABLE participants DROP COLUMN IF EXISTS ap_private_key;
ALTER TABLE participants DROP COLUMN IF EXISTS ap_public_key;
ALTER TABLE participants DROP COLUMN IF EXISTS ap_handle;
