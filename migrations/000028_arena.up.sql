CREATE TYPE arena_status AS ENUM ('pending', 'active', 'completed', 'cancelled');
CREATE TYPE arena_format AS ENUM ('point_counterpoint', 'analysis', 'prediction', 'explanation', 'code_review');

CREATE TABLE arena_battles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    topic TEXT NOT NULL,
    description TEXT,
    agent_a_id UUID NOT NULL REFERENCES participants(id),
    agent_b_id UUID NOT NULL REFERENCES participants(id),
    format arena_format NOT NULL DEFAULT 'point_counterpoint',
    status arena_status NOT NULL DEFAULT 'pending',
    total_rounds INTEGER NOT NULL DEFAULT 5,
    current_round INTEGER NOT NULL DEFAULT 0,
    round_time_limit INTEGER NOT NULL DEFAULT 86400, -- seconds (24h default)
    word_limit INTEGER NOT NULL DEFAULT 500,
    rules TEXT,
    trust_stake FLOAT NOT NULL DEFAULT 0,
    winner_id UUID REFERENCES participants(id),
    voter_count INTEGER NOT NULL DEFAULT 0,
    created_by UUID NOT NULL REFERENCES participants(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE TABLE arena_rounds (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    battle_id UUID NOT NULL REFERENCES arena_battles(id) ON DELETE CASCADE,
    round_number INTEGER NOT NULL,
    round_type VARCHAR(30) NOT NULL DEFAULT 'argument', -- opening, rebuttal, evidence, cross_exam, closing
    agent_a_argument TEXT,
    agent_a_submitted_at TIMESTAMPTZ,
    agent_b_argument TEXT,
    agent_b_submitted_at TIMESTAMPTZ,
    agent_a_argument_score FLOAT NOT NULL DEFAULT 0,
    agent_b_argument_score FLOAT NOT NULL DEFAULT 0,
    agent_a_source_score FLOAT NOT NULL DEFAULT 0,
    agent_b_source_score FLOAT NOT NULL DEFAULT 0,
    agent_a_clarity_score FLOAT NOT NULL DEFAULT 0,
    agent_b_clarity_score FLOAT NOT NULL DEFAULT 0,
    agent_a_total_votes INTEGER NOT NULL DEFAULT 0,
    agent_b_total_votes INTEGER NOT NULL DEFAULT 0,
    round_winner UUID REFERENCES participants(id),
    deadline TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(battle_id, round_number)
);

CREATE TABLE arena_votes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    battle_id UUID NOT NULL REFERENCES arena_battles(id) ON DELETE CASCADE,
    round_id UUID NOT NULL REFERENCES arena_rounds(id) ON DELETE CASCADE,
    voter_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    voted_for UUID NOT NULL REFERENCES participants(id),
    argument_score INTEGER NOT NULL CHECK (argument_score BETWEEN 1 AND 5),
    source_score INTEGER NOT NULL CHECK (source_score BETWEEN 1 AND 5),
    clarity_score INTEGER NOT NULL CHECK (clarity_score BETWEEN 1 AND 5),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(round_id, voter_id)
);

CREATE TABLE arena_comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    battle_id UUID NOT NULL REFERENCES arena_battles(id) ON DELETE CASCADE,
    author_id UUID NOT NULL REFERENCES participants(id),
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_arena_battles_status ON arena_battles(status, created_at DESC);
CREATE INDEX idx_arena_rounds_battle ON arena_rounds(battle_id, round_number);
CREATE INDEX idx_arena_votes_round ON arena_votes(round_id);
CREATE INDEX idx_arena_comments_battle ON arena_comments(battle_id, created_at);
