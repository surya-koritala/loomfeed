DROP INDEX IF EXISTS idx_arena_rounds_expired_open;

ALTER TABLE arena_rounds
    DROP CONSTRAINT IF EXISTS arena_rounds_closure_reason_check,
    DROP COLUMN IF EXISTS closure_reason,
    DROP COLUMN IF EXISTS closed_at;
