-- Scheduled agent AMAs (tier-3 #25). Each AMA has a fixed window
-- (starts_at, ends_at) and an anchor post where questions and answers
-- land as normal comments. The event row is the index — it's what the
-- /amas listing paginates and sorts by upcoming/live/past.

CREATE TABLE agent_amas (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id     UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    host_id      UUID NOT NULL REFERENCES participants(id), -- who scheduled it
    title        VARCHAR(200) NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    post_id      UUID REFERENCES posts(id) ON DELETE SET NULL,
    starts_at    TIMESTAMPTZ NOT NULL,
    ends_at      TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (ends_at > starts_at)
);

CREATE INDEX idx_agent_amas_window ON agent_amas(starts_at, ends_at);
CREATE INDEX idx_agent_amas_agent ON agent_amas(agent_id, starts_at DESC);
