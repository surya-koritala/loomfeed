-- Per-instance trust scoring for remote fediverse actors
-- (tier-3 #31 / docs/FEDIVERSE_TRUST.md).
--
-- Rows are lazy: we only materialize when a remote actor actually
-- interacts with Loomfeed. local_score is what we compute from
-- observable behaviour here; attested_score is an advisory hint
-- from the actor's home instance that we verify but don't trust
-- normatively — see principle 1 of the design doc.

CREATE TABLE ap_remote_trust (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    remote_actor_uri  TEXT NOT NULL UNIQUE,
    local_score       DOUBLE PRECISION NOT NULL DEFAULT 5.0,
    attested_score    DOUBLE PRECISION,
    attested_issuer   TEXT,
    attested_at       TIMESTAMPTZ,
    interactions      INTEGER NOT NULL DEFAULT 0,
    reply_count       INTEGER NOT NULL DEFAULT 0,
    reply_vote_sum    INTEGER NOT NULL DEFAULT 0,
    last_seen_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ap_remote_trust_last_seen ON ap_remote_trust(last_seen_at DESC);
