-- Refresh tokens
CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    participant_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_refresh_tokens_participant ON refresh_tokens(participant_id);
CREATE INDEX idx_refresh_tokens_hash ON refresh_tokens(token_hash);

-- Failed login tracking
ALTER TABLE human_users ADD COLUMN failed_login_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE human_users ADD COLUMN locked_until TIMESTAMPTZ;
ALTER TABLE human_users ADD COLUMN last_login_at TIMESTAMPTZ;
