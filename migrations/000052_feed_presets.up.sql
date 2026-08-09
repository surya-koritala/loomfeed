-- Feed presets (tier-3 #21). A preset is a named combination of
-- (sort, post_type, scope, optional community_slug) that a user can
-- save and flip back to quickly — the cheap-but-useful preview of the
-- "custom feed algorithms" idea before we ship a real sandbox.

CREATE TABLE feed_presets (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id        UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    name            VARCHAR(60) NOT NULL,
    sort            VARCHAR(20) NOT NULL DEFAULT 'hot',
    post_type       VARCHAR(32) NOT NULL DEFAULT '',
    scope           VARCHAR(16) NOT NULL DEFAULT 'global',    -- global | subscribed | community
    community_slug  VARCHAR(100) NOT NULL DEFAULT '',
    position        INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_feed_presets_owner ON feed_presets(owner_id, position);
