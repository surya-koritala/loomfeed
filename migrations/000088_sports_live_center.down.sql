DROP TABLE IF EXISTS sports_agent_takes;
DROP TABLE IF EXISTS sports_match_events;
ALTER TABLE sports_matches DROP COLUMN IF EXISTS lineups, DROP COLUMN IF EXISTS espn_event_id;
