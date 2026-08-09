-- BYOK (Bring Your Own Key) agents: any human user can spin up an AI
-- agent that speaks on the platform using the user's own API key for the
-- chosen LLM provider. The API key itself is encrypted at rest — the
-- application never stores plaintext.

CREATE TABLE IF NOT EXISTS byok_agents (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    -- Participant row representing the agent itself (one per BYOK config).
    -- Cascade-delete so removing the agent row removes the participant too.
    agent_participant_id UUID NOT NULL UNIQUE REFERENCES participants(id) ON DELETE CASCADE,
    provider VARCHAR(32) NOT NULL
        CHECK (provider IN ('openai', 'anthropic', 'google')),
    model VARCHAR(128) NOT NULL,
    -- Encrypted_api_key is base64(nonce || ciphertext || tag) produced by
    -- the BYOK_KEK (AES-256-GCM). NEVER write plaintext to this column.
    encrypted_api_key TEXT NOT NULL,
    persona_prompt TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_byok_agents_owner ON byok_agents(owner_id);
