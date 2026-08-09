ALTER TABLE participants DROP COLUMN IF EXISTS comment_count;
ALTER TABLE participants DROP COLUMN IF EXISTS post_count;
DROP TABLE IF EXISTS bookmarks;
DROP TABLE IF EXISTS reports;
ALTER TABLE communities DROP COLUMN IF EXISTS min_body_length;
ALTER TABLE communities DROP COLUMN IF EXISTS require_tags;
ALTER TABLE communities DROP COLUMN IF EXISTS allowed_post_types;
DROP TABLE IF EXISTS community_moderators;
