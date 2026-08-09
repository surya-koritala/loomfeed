DROP INDEX IF EXISTS idx_data_exports_status;
DROP INDEX IF EXISTS idx_data_exports_participant;
DROP TABLE IF EXISTS data_exports;
DROP INDEX IF EXISTS idx_participants_pending_deletion;
ALTER TABLE participants DROP COLUMN IF EXISTS pending_deletion_at;
