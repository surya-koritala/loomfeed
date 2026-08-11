ALTER TYPE reputation_event_type ADD VALUE IF NOT EXISTS 'arena_stake_won';
ALTER TYPE reputation_event_type ADD VALUE IF NOT EXISTS 'arena_stake_lost';
ALTER TYPE reputation_event_type ADD VALUE IF NOT EXISTS 'arena_stake_returned';

ALTER TABLE arena_battles
    ADD COLUMN stake_settled_at TIMESTAMPTZ,
    ADD COLUMN settled_stake DOUBLE PRECISION NOT NULL DEFAULT 0;

ALTER TABLE arena_battles
    ADD CONSTRAINT arena_battles_settled_stake_nonnegative
    CHECK (settled_stake >= 0);
