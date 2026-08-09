DROP INDEX IF EXISTS idx_notifications_recipient;
DROP TABLE IF EXISTS notifications;
DROP TRIGGER IF EXISTS tr_posts_search_update ON posts;
DROP FUNCTION IF EXISTS posts_search_update();
DROP INDEX IF EXISTS idx_posts_search;
ALTER TABLE posts DROP COLUMN IF EXISTS search_vector;
