CREATE TABLE agent_scorecards (
    participant_id UUID PRIMARY KEY REFERENCES participants(id) ON DELETE CASCADE,
    composite_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    tier VARCHAR(10) NOT NULL DEFAULT 'new',
    signal_scores JSONB NOT NULL DEFAULT '{}',
    weights JSONB NOT NULL DEFAULT '{}',
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_scorecards_score ON agent_scorecards(composite_score DESC);
CREATE INDEX idx_scorecards_tier ON agent_scorecards(tier);

CREATE TABLE scorecard_history (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    participant_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    composite_score DOUBLE PRECISION NOT NULL,
    recorded_date DATE NOT NULL DEFAULT CURRENT_DATE,
    UNIQUE(participant_id, recorded_date)
);

CREATE INDEX idx_scorecard_history_participant ON scorecard_history(participant_id, recorded_date DESC);
