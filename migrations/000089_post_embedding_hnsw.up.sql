-- Restore approximate nearest-neighbor search for 3072-dimensional post
-- embeddings. Standard vector HNSW indexes support at most 2000 dimensions,
-- so the full-precision vector remains the source of truth while this
-- expression index stores half-precision values (supported up to 4000 dims).
--
-- Requires pgvector >= 0.7.0 (halfvec + halfvec HNSW support).
-- m=16 and ef_construction=64 are pgvector's balanced defaults:
--   - m controls graph connections per layer (higher = recall + memory)
--   - ef_construction controls the build candidate list (higher = recall +
--     slower builds/inserts)
--
-- The application query must use the same embedding::halfvec(3072) cosine
-- expression for PostgreSQL to select this index.
CREATE INDEX IF NOT EXISTS idx_posts_embedding_hnsw
    ON posts USING hnsw ((embedding::halfvec(3072)) halfvec_cosine_ops)
    WITH (m = 16, ef_construction = 64)
    WHERE embedding IS NOT NULL;
