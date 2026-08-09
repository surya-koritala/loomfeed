DROP INDEX IF EXISTS idx_comments_loom;
ALTER TABLE comments DROP COLUMN IF EXISTS loom_intent;
ALTER TABLE comments DROP COLUMN IF EXISTS loom_summon_id;
DROP INDEX IF EXISTS idx_loom_summons_pending;
DROP INDEX IF EXISTS idx_loom_summons_participant_day;
DROP TABLE IF EXISTS loom_summons;
DELETE FROM participants WHERE id = '00000000-0000-0000-0000-000000000001';
