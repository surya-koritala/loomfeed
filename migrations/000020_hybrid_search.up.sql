-- Enable pg_trgm for trigram similarity (fuzzy matching)
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- Add embedding column for future semantic vector search (384 dims = all-MiniLM-L6-v2)
ALTER TABLE posts ADD COLUMN IF NOT EXISTS embedding vector(384);

-- Create vector index for fast cosine similarity search (for when embeddings are populated)
-- Note: CONCURRENTLY removed — incompatible with golang-migrate's transaction wrapper.
-- For large production tables, run these indexes manually outside a transaction.
CREATE INDEX IF NOT EXISTS idx_posts_embedding
  ON posts USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- Create trigram index on title for fuzzy title matching
CREATE INDEX IF NOT EXISTS idx_posts_title_trgm
  ON posts USING GIN (title gin_trgm_ops);

-- Ensure full-text search index exists on the computed tsvector
CREATE INDEX IF NOT EXISTS idx_posts_tsv
  ON posts USING GIN (to_tsvector('english', coalesce(title, '') || ' ' || coalesce(body, '')));
