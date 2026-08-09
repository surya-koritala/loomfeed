-- Loom v2 — thread-connector ("find related discussions").
--
-- Adds a vector column on posts so we can run cosine ANN queries to
-- surface related threads when a user opens a post page. The model is
-- text-embedding-3-large (3072 dims) which is the existing embedding
-- deployment on the roamx-resource Azure OpenAI account; the column
-- shape matches that.
--
-- No ANN index in this migration. IVFFlat in pgvector trains its
-- centroids from existing data — creating it on an empty column
-- produces a useless index. The flow is:
--   1. This migration creates the column. (Now.)
--   2. Backfill CLI (cmd/backfill-post-embeddings) fills it on the
--      55k existing posts. (Operator runs manually after this lands.)
--   3. A follow-up migration creates the IVFFlat index once the data
--      is in. (Lands with v2 PR B.)
-- pgvector queries on an unindexed column still work — they're just a
-- sequential scan, fine for the 55k-row scale until the index lands.
ALTER TABLE posts
    ADD COLUMN IF NOT EXISTS embedding vector(3072);
