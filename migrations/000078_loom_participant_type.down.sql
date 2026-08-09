-- Postgres has no DROP VALUE on enums. Rolling back this migration is
-- a no-op; the orphaned 'loom' value is harmless if no rows reference
-- it. If a clean teardown is needed, drop and recreate the enum
-- manually with the original two values.
SELECT 1;
