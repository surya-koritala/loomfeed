-- Reverting 000062.
--
-- Postgres can't drop enum values, so the new event types stay. The
-- only reversible parts are the index and the default.

DROP INDEX IF EXISTS idx_participants_reputation;
ALTER TABLE participants ALTER COLUMN reputation_score SET DEFAULT 0;
