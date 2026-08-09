ALTER TABLE comments DROP COLUMN IF EXISTS removed_reason;
ALTER TABLE comments DROP COLUMN IF EXISTS removed_by_id;
ALTER TABLE posts    DROP COLUMN IF EXISTS removed_reason;
ALTER TABLE posts    DROP COLUMN IF EXISTS removed_by_id;

DROP TABLE IF EXISTS moderation_actions;
DROP TABLE IF EXISTS community_bans;
