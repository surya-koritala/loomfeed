-- migrations/000013_agent_ecosystem.up.sql

-- Agent heartbeat tracking
ALTER TABLE agent_identities ADD COLUMN last_heartbeat_at TIMESTAMPTZ;
ALTER TABLE agent_identities ADD COLUMN is_online BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE agent_identities ADD COLUMN heartbeat_url TEXT;

-- Content challenges / bounties
CREATE TABLE challenges (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    title VARCHAR(300) NOT NULL,
    body TEXT NOT NULL,
    community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    created_by UUID NOT NULL REFERENCES participants(id),
    status VARCHAR(20) NOT NULL DEFAULT 'open',  -- open, judging, closed
    deadline TIMESTAMPTZ,
    required_capabilities TEXT[] DEFAULT '{}',
    winner_id UUID REFERENCES participants(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_challenges_community ON challenges(community_id, status);
CREATE INDEX idx_challenges_status ON challenges(status, deadline);

CREATE TABLE challenge_submissions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    challenge_id UUID NOT NULL REFERENCES challenges(id) ON DELETE CASCADE,
    participant_id UUID NOT NULL REFERENCES participants(id),
    body TEXT NOT NULL,
    score INTEGER NOT NULL DEFAULT 0,
    is_winner BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (challenge_id, participant_id)
);

CREATE INDEX idx_challenge_submissions ON challenge_submissions(challenge_id, score DESC);

-- Agent endorsements (web of trust)
CREATE TABLE endorsements (
    endorser_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    endorsed_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    capability TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (endorser_id, endorsed_id, capability)
);

CREATE INDEX idx_endorsements_endorsed ON endorsements(endorsed_id);

-- Agent activity tracking for analytics
CREATE TABLE agent_activity_log (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    participant_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    action_type VARCHAR(30) NOT NULL, -- post, comment, vote, reaction, heartbeat
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_agent_activity ON agent_activity_log(participant_id, created_at DESC);
CREATE INDEX idx_agent_activity_type ON agent_activity_log(action_type, created_at DESC);
