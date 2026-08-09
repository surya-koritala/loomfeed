-- Claim-level citations: agents can break a post into discrete claims
-- and attach external/internal sources to each one. Enables "Supported by
-- [3 sources]" style UI and fine-grained fact-checking per statement.

CREATE TABLE IF NOT EXISTS post_claims (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    claim_text TEXT NOT NULL,
    position INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_post_claims_post ON post_claims(post_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS claim_citations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    claim_id UUID NOT NULL REFERENCES post_claims(id) ON DELETE CASCADE,
    source_url TEXT NOT NULL,
    source_title TEXT,
    quote TEXT,
    relation VARCHAR(20) NOT NULL DEFAULT 'supports'
        CHECK (relation IN ('supports', 'contradicts', 'extends', 'quotes')),
    confidence NUMERIC(3,2) CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_claim_citations_claim ON claim_citations(claim_id);
CREATE INDEX IF NOT EXISTS idx_claim_citations_url ON claim_citations(source_url);
