DROP TRIGGER IF EXISTS tr_comments_updated_at ON comments;
DROP TRIGGER IF EXISTS tr_posts_updated_at ON posts;
DROP TRIGGER IF EXISTS tr_communities_updated_at ON communities;
DROP TRIGGER IF EXISTS tr_participants_updated_at ON participants;
DROP FUNCTION IF EXISTS update_updated_at();

DROP TABLE IF EXISTS quality_gates;
DROP TABLE IF EXISTS reputation_events;
DROP TABLE IF EXISTS citation_edges;
DROP TABLE IF EXISTS provenances;
DROP TABLE IF EXISTS votes;
DROP TABLE IF EXISTS comments;
DROP TABLE IF EXISTS posts;
DROP TABLE IF EXISTS community_subscriptions;
DROP TABLE IF EXISTS communities;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS agent_identities;
DROP TABLE IF EXISTS human_users;
DROP TABLE IF EXISTS participants;

DROP TYPE IF EXISTS reputation_event_type;
DROP TYPE IF EXISTS citation_type;
DROP TYPE IF EXISTS generation_method;
DROP TYPE IF EXISTS target_type;
DROP TYPE IF EXISTS vote_direction;
DROP TYPE IF EXISTS content_type;
DROP TYPE IF EXISTS agent_policy;
DROP TYPE IF EXISTS protocol_type;
DROP TYPE IF EXISTS participant_type;

DROP EXTENSION IF EXISTS "vector";
DROP EXTENSION IF EXISTS "uuid-ossp";
