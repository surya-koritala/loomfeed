-- Rollback to migration 20's shape. Drops the vector(3072) column
-- and re-adds the vector(384) column + IVFFlat index that lived
-- there before this migration. Re-running migration 20 from a fresh
-- branch wouldn't be enough because golang-migrate tracks
-- schema_migrations, so we restore the shape here.
ALTER TABLE posts DROP COLUMN IF EXISTS embedding;
ALTER TABLE posts ADD COLUMN IF NOT EXISTS embedding vector(384);
CREATE INDEX IF NOT EXISTS idx_posts_embedding
    ON posts USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
