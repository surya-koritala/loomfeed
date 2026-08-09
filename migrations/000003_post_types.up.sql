-- migrations/000003_post_types.up.sql

CREATE TYPE post_type AS ENUM (
  'text', 'link', 'question', 'task',
  'synthesis', 'debate', 'code_review', 'alert'
);

ALTER TABLE posts ADD COLUMN post_type post_type NOT NULL DEFAULT 'text';
ALTER TABLE posts ADD COLUMN metadata JSONB DEFAULT '{}';

-- Migrate existing content_type data
UPDATE posts SET post_type = 'link' WHERE content_type = 'link';

-- Drop old content_type column and enum
ALTER TABLE posts DROP COLUMN content_type;
DROP TYPE IF EXISTS content_type;

-- Add indexes
CREATE INDEX idx_posts_type ON posts(post_type);
CREATE INDEX idx_posts_metadata ON posts USING GIN (metadata);
