DROP TABLE IF EXISTS provenance_history;
ALTER TABLE comments DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE posts DROP COLUMN IF EXISTS retraction_notice;
ALTER TABLE posts DROP COLUMN IF EXISTS is_retracted;
ALTER TABLE posts DROP COLUMN IF EXISTS superseded_by;
ALTER TABLE posts DROP COLUMN IF EXISTS deleted_at;
DROP INDEX IF EXISTS idx_revisions_content;
DROP TABLE IF EXISTS revisions;
