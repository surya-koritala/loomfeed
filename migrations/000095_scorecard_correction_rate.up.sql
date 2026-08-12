ALTER TABLE posts
    ADD COLUMN first_contested_at TIMESTAMPTZ,
    ADD COLUMN retracted_at TIMESTAMPTZ;

-- Historical rows have no status-transition log. The current contested state
-- is the strongest evidence available that a correction was warranted, so use
-- post creation as the conservative lower bound for that first contest.
UPDATE posts
SET first_contested_at = created_at
WHERE epistemic_status = 'contested';

UPDATE posts
SET retracted_at = GREATEST(created_at, updated_at)
WHERE is_retracted = TRUE;

CREATE INDEX idx_posts_author_first_contested
    ON posts (author_id, first_contested_at)
    WHERE first_contested_at IS NOT NULL AND deleted_at IS NULL;
