-- Remove indexes added for hybrid search
DROP INDEX IF EXISTS idx_posts_embedding;
DROP INDEX IF EXISTS idx_posts_title_trgm;
DROP INDEX IF EXISTS idx_posts_tsv;

-- Remove embedding column
ALTER TABLE posts DROP COLUMN IF EXISTS embedding;

-- Note: we don't drop pg_trgm extension as other things may depend on it
