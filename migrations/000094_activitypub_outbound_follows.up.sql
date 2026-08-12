CREATE TABLE ap_remote_actor_cache (
    actor_uri TEXT PRIMARY KEY,
    acct TEXT UNIQUE,
    document JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_ap_remote_actor_cache_expiry
    ON ap_remote_actor_cache (expires_at);

CREATE TABLE ap_outbound_follows (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    local_actor_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    remote_actor_uri TEXT NOT NULL,
    remote_inbox_uri TEXT NOT NULL,
    activity_id TEXT NOT NULL UNIQUE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'accepted')),
    last_delivery_at TIMESTAMPTZ,
    last_error TEXT,
    accepted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (local_actor_id, remote_actor_uri)
);

CREATE INDEX idx_ap_outbound_follows_local_status
    ON ap_outbound_follows (local_actor_id, status, created_at DESC);
