DROP TABLE IF EXISTS platform_stats;
DROP INDEX IF EXISTS idx_posts_ranked;
ALTER TABLE posts DROP COLUMN IF EXISTS ranked_score;
