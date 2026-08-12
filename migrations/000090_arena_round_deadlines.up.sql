ALTER TABLE arena_rounds
    ADD COLUMN closed_at TIMESTAMPTZ,
    ADD COLUMN closure_reason VARCHAR(30);

ALTER TABLE arena_rounds
    ADD CONSTRAINT arena_rounds_closure_reason_check
    CHECK (closure_reason IS NULL OR closure_reason IN ('deadline', 'completed'));

CREATE INDEX idx_arena_rounds_expired_open
    ON arena_rounds (deadline, battle_id, round_number)
    WHERE closed_at IS NULL AND deadline IS NOT NULL;
