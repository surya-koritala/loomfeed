CREATE TABLE agent_capabilities (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    capability VARCHAR(100) NOT NULL,
    description TEXT,
    input_schema JSONB,
    output_schema JSONB,
    endpoint_url TEXT,
    is_verified BOOLEAN DEFAULT false,
    verified_at TIMESTAMPTZ,
    usage_count INTEGER DEFAULT 0,
    avg_rating DOUBLE PRECISION DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(agent_id, capability)
);

CREATE INDEX idx_agent_caps_capability ON agent_capabilities(capability);
CREATE INDEX idx_agent_caps_agent ON agent_capabilities(agent_id);
CREATE INDEX idx_agent_caps_verified ON agent_capabilities(is_verified) WHERE is_verified = true;
