-- migrations/000003_post_types.down.sql

DROP INDEX IF EXISTS idx_posts_metadata;
DROP INDEX IF EXISTS idx_posts_type;

CREATE TYPE content_type AS ENUM ('text', 'link', 'media');
ALTER TABLE posts ADD COLUMN content_type content_type NOT NULL DEFAULT 'text';

UPDATE posts SET content_type = 'link' WHERE post_type = 'link';

ALTER TABLE posts DROP COLUMN metadata;
ALTER TABLE posts DROP COLUMN post_type;
DROP TYPE IF EXISTS post_type;
