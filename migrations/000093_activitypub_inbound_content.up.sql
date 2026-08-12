ALTER TYPE participant_type ADD VALUE IF NOT EXISTS 'remote';

CREATE TABLE ap_remote_actors (
    participant_id UUID PRIMARY KEY REFERENCES participants(id) ON DELETE CASCADE,
    actor_uri TEXT NOT NULL UNIQUE,
    preferred_username VARCHAR(100) NOT NULL DEFAULT '',
    actor_type VARCHAR(50) NOT NULL DEFAULT 'Person',
    inbox_uri TEXT NOT NULL DEFAULT '',
    instance_host TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE comments
    ADD COLUMN federated_object_id TEXT,
    ADD COLUMN federated_activity_id TEXT,
    ADD COLUMN federated_actor_uri TEXT;

CREATE UNIQUE INDEX idx_comments_federated_object
    ON comments (federated_object_id) WHERE federated_object_id IS NOT NULL;
CREATE UNIQUE INDEX idx_comments_federated_activity
    ON comments (federated_activity_id) WHERE federated_activity_id IS NOT NULL;

ALTER TABLE votes
    ADD COLUMN weight DOUBLE PRECISION NOT NULL DEFAULT 1,
    ADD COLUMN federated_activity_id TEXT,
    ADD COLUMN federated_actor_uri TEXT,
    ADD CONSTRAINT votes_weight_range CHECK (weight > 0 AND weight <= 1);

CREATE UNIQUE INDEX idx_votes_federated_activity
    ON votes (federated_activity_id) WHERE federated_activity_id IS NOT NULL;
