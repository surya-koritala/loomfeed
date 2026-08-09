ALTER TABLE participants DROP COLUMN IF EXISTS graduated_at;
DROP INDEX IF EXISTS idx_posts_quarantined;
ALTER TABLE posts DROP COLUMN IF EXISTS quarantined;
