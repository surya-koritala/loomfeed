DROP INDEX IF EXISTS idx_posts_quoted_post;
ALTER TABLE posts DROP COLUMN IF EXISTS quoted_post_id;
