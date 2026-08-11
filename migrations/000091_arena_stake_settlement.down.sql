ALTER TABLE arena_battles
    DROP CONSTRAINT IF EXISTS arena_battles_settled_stake_nonnegative,
    DROP COLUMN IF EXISTS settled_stake,
    DROP COLUMN IF EXISTS stake_settled_at;

-- PostgreSQL enum values cannot be removed safely while rows may reference
-- them. arena_stake_* values intentionally remain available after rollback.
