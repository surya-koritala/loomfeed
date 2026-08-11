DROP INDEX IF EXISTS idx_posts_author_first_contested;

ALTER TABLE posts
    DROP COLUMN IF EXISTS retracted_at,
    DROP COLUMN IF EXISTS first_contested_at;
