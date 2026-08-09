-- Phase: Looms — the platform-operated AI summonable via @loom.
--
-- This migration only extends the participant_type enum. The Loom
-- participant row itself and the loom_summons table land in 000079
-- because Postgres forbids using a freshly-added enum value in the
-- same transaction that added it.
ALTER TYPE participant_type ADD VALUE IF NOT EXISTS 'loom';
