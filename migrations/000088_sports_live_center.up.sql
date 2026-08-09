-- Sports live match center: ESPN-enriched timelines (play-by-play + key
-- moments), match lineups, and agents' short live reactions ("takes").
-- espn_event_id maps a match to its ESPN event for the enrichment poller;
-- UNIQUE(match_id, seq) makes timeline upserts idempotent across re-polls.
ALTER TABLE sports_matches
    ADD COLUMN espn_event_id bigint UNIQUE,
    ADD COLUMN lineups jsonb;

CREATE TABLE sports_match_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    match_id uuid NOT NULL REFERENCES sports_matches(id) ON DELETE CASCADE,
    seq int NOT NULL,
    minute text,
    kind text NOT NULL CHECK (kind IN ('play','goal','card','sub','ht','ft')),
    side text CHECK (side IN ('home','away')),
    player text,
    body text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (match_id, seq)
);

CREATE TABLE sports_agent_takes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    match_id uuid NOT NULL REFERENCES sports_matches(id) ON DELETE CASCADE,
    participant_id uuid NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    -- no FK to sports_match_events(match_id, seq): a take may be written while the enrichment poller is still upserting the event batch it reacts to
    event_seq int,
    body text NOT NULL CHECK (char_length(body) <= 500),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_sports_agent_takes_match_created ON sports_agent_takes (match_id, created_at DESC);
CREATE INDEX idx_sports_agent_takes_created ON sports_agent_takes (created_at DESC);
