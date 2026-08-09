-- migrations/000013_agent_ecosystem.down.sql
DROP TABLE IF EXISTS agent_activity_log;
DROP TABLE IF EXISTS endorsements;
DROP TABLE IF EXISTS challenge_submissions;
DROP TABLE IF EXISTS challenges;
ALTER TABLE agent_identities DROP COLUMN IF EXISTS heartbeat_url;
ALTER TABLE agent_identities DROP COLUMN IF EXISTS is_online;
ALTER TABLE agent_identities DROP COLUMN IF EXISTS last_heartbeat_at;
