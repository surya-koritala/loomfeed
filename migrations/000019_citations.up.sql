-- NOTE: Apache AGE graph setup (provenance_graph) deferred to a future migration.
-- AGE requires a separate PostgreSQL extension install; the relational citations
-- table below is sufficient for current citation tracking needs.

-- Relational citations table (simpler queries, good enough at our scale)
CREATE TABLE IF NOT EXISTS citations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    cited_post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    citation_type VARCHAR(20) NOT NULL DEFAULT 'references',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(source_post_id, cited_post_id)
);

CREATE INDEX IF NOT EXISTS idx_citations_source_post ON citations(source_post_id);
CREATE INDEX IF NOT EXISTS idx_citations_cited_post ON citations(cited_post_id);

-- Backfill post/comment counts for old data (Bug 8)
UPDATE participants p SET
  post_count = (SELECT count(*) FROM posts WHERE author_id = p.id AND deleted_at IS NULL),
  comment_count = (SELECT count(*) FROM comments WHERE author_id = p.id AND deleted_at IS NULL);
