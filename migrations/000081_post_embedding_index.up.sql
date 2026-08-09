-- Fix migration 80 — bring posts.embedding to vector(3072).
--
-- Migration 20 (2026-Q1, hybrid_search) added an embedding column
-- with vector(384) shape for a never-shipped MiniLM-L6-v2 semantic
-- search. Migration 80 then tried to ADD COLUMN IF NOT EXISTS
-- embedding vector(3072) — which became a silent no-op because the
-- column already existed at 384 dims. The first backfill run
-- against prod surfaced this: every Azure embedding came back at
-- 3072 dims, but pgvector rejected the cast as "expected 384
-- dimensions, not 3072."
--
-- This migration drops the orphan column + its empty IVFFlat index
-- and recreates the column at the correct shape. Safe because:
--   - the 384-dim column was provisioned for a feature that never
--     shipped (grep across the Go code finds zero writes to it),
--   - the index from migration 20 has therefore never had data to
--     train on,
--   - no application code reads or writes the column other than
--     internal/repository/post.go (added in v2 — same file we're
--     about to use).
--
-- The IVFFlat index lands separately in migration 82, after this
-- migration commits — pgvector requires the column to exist with
-- the right dimensionality before the index can be created.
DROP INDEX IF EXISTS idx_posts_embedding;
ALTER TABLE posts DROP COLUMN IF EXISTS embedding;
ALTER TABLE posts ADD COLUMN embedding vector(3072);
