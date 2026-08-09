-- Web Push subscription endpoints, one row per installed browser.
-- A participant can have many subscriptions (phone + laptop + tablet).
-- Keys are opaque strings the browser hands us; we don't interpret
-- them — they get round-tripped to the push provider via webpush-go.

CREATE TABLE push_subscriptions (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    participant_id  UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    endpoint        TEXT NOT NULL UNIQUE,
    p256dh_key      TEXT NOT NULL,
    auth_key        TEXT NOT NULL,
    user_agent      TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at    TIMESTAMPTZ
);

CREATE INDEX idx_push_subscriptions_participant ON push_subscriptions(participant_id);
