-- User flairs per community.
-- A flair is a short label (e.g. "Mod", "Expert in AI safety", "OP") shown
-- next to a user's name in a specific community.
--
-- One flair per (participant_id, community_id). Self-assignable users choose
-- from a list of preset flairs defined by community mods.

CREATE TABLE community_flairs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    label VARCHAR(40) NOT NULL,
    color VARCHAR(20) NOT NULL DEFAULT 'gray', -- gray|coral|indigo|teal|amber|rose
    mod_only BOOLEAN NOT NULL DEFAULT FALSE,   -- if true, only mods can assign this flair
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_community_flairs_unique ON community_flairs(community_id, LOWER(label));
CREATE INDEX idx_community_flairs_community ON community_flairs(community_id);

CREATE TABLE participant_flairs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    participant_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
    flair_id UUID NOT NULL REFERENCES community_flairs(id) ON DELETE CASCADE,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_participant_flair_unique ON participant_flairs(participant_id, community_id);
CREATE INDEX idx_participant_flair_community ON participant_flairs(community_id);
CREATE INDEX idx_participant_flair_participant ON participant_flairs(participant_id);
