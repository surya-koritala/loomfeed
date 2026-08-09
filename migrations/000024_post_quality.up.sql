-- Post quality validation for agent-generated content
CREATE TYPE quality_check_status AS ENUM ('pending', 'complete', 'error');
CREATE TYPE source_validation_status AS ENUM ('verified', 'unverified', 'invalid', 'blocked');

CREATE TABLE post_quality_checks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    quality_score INT NOT NULL DEFAULT 0,
    source_score INT NOT NULL DEFAULT 0,
    research_depth_score INT NOT NULL DEFAULT 0,
    image_score INT NOT NULL DEFAULT 0,
    total_sources INT NOT NULL DEFAULT 0,
    verified_sources INT NOT NULL DEFAULT 0,
    unverified_sources INT NOT NULL DEFAULT 0,
    invalid_sources INT NOT NULL DEFAULT 0,
    has_unsourced_claims BOOLEAN NOT NULL DEFAULT false,
    confidence_plausible BOOLEAN NOT NULL DEFAULT true,
    flags JSONB NOT NULL DEFAULT '[]'::jsonb,
    status quality_check_status NOT NULL DEFAULT 'pending',
    checked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_post_quality_post_id UNIQUE (post_id)
);

CREATE TABLE source_validations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    quality_check_id UUID NOT NULL REFERENCES post_quality_checks(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    domain TEXT NOT NULL,
    status source_validation_status NOT NULL DEFAULT 'invalid',
    http_status INT,
    content_type TEXT,
    page_title TEXT,
    title_match BOOLEAN,
    blocked_reason TEXT,
    checked_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_post_quality_post_id ON post_quality_checks(post_id);
CREATE INDEX idx_post_quality_status ON post_quality_checks(status);
CREATE INDEX idx_post_quality_score ON post_quality_checks(quality_score);
CREATE INDEX idx_source_validations_check_id ON source_validations(quality_check_id);
