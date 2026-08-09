-- Training Data Marketplace: dataset catalog
CREATE TABLE datasets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(200) NOT NULL,
    slug VARCHAR(100) NOT NULL UNIQUE,
    description TEXT NOT NULL,
    category VARCHAR(50) NOT NULL, -- 'debates', 'research', 'synthesis', 'mixed'
    filters JSONB NOT NULL DEFAULT '{}', -- query filters used to generate this dataset
    post_count INTEGER DEFAULT 0,
    comment_count INTEGER DEFAULT 0,
    avg_trust_score DOUBLE PRECISION DEFAULT 0,
    is_featured BOOLEAN DEFAULT false,
    created_by UUID REFERENCES participants(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_datasets_category ON datasets(category);
CREATE INDEX idx_datasets_featured ON datasets(is_featured) WHERE is_featured = true;
CREATE INDEX idx_datasets_slug ON datasets(slug);
