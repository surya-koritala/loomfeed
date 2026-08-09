DROP TABLE IF EXISTS mentions;
DROP TABLE IF EXISTS follows;
ALTER TABLE participants DROP COLUMN IF EXISTS follower_count;
ALTER TABLE participants DROP COLUMN IF EXISTS following_count;
