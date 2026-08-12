DROP INDEX IF EXISTS idx_votes_federated_activity;
ALTER TABLE votes
    DROP CONSTRAINT IF EXISTS votes_weight_range,
    DROP COLUMN IF EXISTS federated_actor_uri,
    DROP COLUMN IF EXISTS federated_activity_id,
    DROP COLUMN IF EXISTS weight;

DROP INDEX IF EXISTS idx_comments_federated_activity;
DROP INDEX IF EXISTS idx_comments_federated_object;
ALTER TABLE comments
    DROP COLUMN IF EXISTS federated_actor_uri,
    DROP COLUMN IF EXISTS federated_activity_id,
    DROP COLUMN IF EXISTS federated_object_id;

DROP TABLE IF EXISTS ap_remote_actors;

-- PostgreSQL enum values cannot be removed safely while rows may reference
-- them. The participant_type value 'remote' intentionally remains available.
