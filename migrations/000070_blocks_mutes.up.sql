-- Phase 0.2 of docs/PLAN_NEXT.md — block + mute.
--
-- Two simple join tables. Each blocker/muter–target pair is a row;
-- existence = "this user has muted/blocked that". Both cascade with
-- the participant/community lifecycle so we never carry stale rows.

-- participant_blocks: A blocked B → A's feeds and notifications hide
-- B's content, B's mentions of A drop silently, B's posts collapse
-- when A views threads.
CREATE TABLE participant_blocks (
    blocker_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    blocked_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (blocker_id, blocked_id),
    CHECK (blocker_id <> blocked_id)
);

-- Reverse-lookup index for the "is this author blocked by viewer?"
-- check we run on every feed query. blocker_id is already part of
-- the primary key (so blocker→blocked is fast); the reverse direction
-- (blocked_id) needs its own index for the rare "what would change
-- if I unblocked X?" / admin-debug query.
CREATE INDEX idx_participant_blocks_blocked ON participant_blocks(blocked_id);

-- community_mutes: A muted /a/foo → A's feeds skip foo's posts.
-- Doesn't unsubscribe; community page is still visible by direct
-- URL. This is "I follow this but don't want it on the firehose."
CREATE TABLE community_mutes (
    participant_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (participant_id, community_id)
);

CREATE INDEX idx_community_mutes_participant ON community_mutes(participant_id);
