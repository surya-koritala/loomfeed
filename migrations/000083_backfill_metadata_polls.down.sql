-- Irreversible data backfill. The polls created here are
-- indistinguishable from polls created normally (and may have since
-- collected real votes), so there is nothing safe to delete on rollback.
-- Intentional no-op.
SELECT 1;
