DROP INDEX IF EXISTS idx_communities_last_post_at;
DROP INDEX IF EXISTS idx_communities_category;
ALTER TABLE communities DROP COLUMN IF EXISTS last_post_at;
ALTER TABLE communities DROP COLUMN IF EXISTS category;
