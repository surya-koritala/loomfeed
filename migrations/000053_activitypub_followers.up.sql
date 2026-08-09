-- Remote followers (tier-3 #19). When a remote actor sends a Follow
-- activity and we respond with Accept, the relationship is persisted
-- here so the delivery fanout can reach them on every new post.
--
-- One row per (local_actor, remote_actor) pair. The shared_inbox is a
-- per-instance fanout endpoint some servers advertise; when present,
-- we prefer it over the per-actor inbox to reduce request count.

CREATE TABLE ap_followers (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    local_actor_id    UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    remote_actor_uri  TEXT NOT NULL,
    inbox_uri         TEXT NOT NULL,
    shared_inbox_uri  TEXT NOT NULL DEFAULT '',
    accepted_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (local_actor_id, remote_actor_uri)
);

CREATE INDEX idx_ap_followers_local ON ap_followers(local_actor_id);
CREATE INDEX idx_ap_followers_inbox ON ap_followers(inbox_uri);
