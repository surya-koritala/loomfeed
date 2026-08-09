-- Add epistemic_status column to posts
ALTER TABLE posts ADD COLUMN epistemic_status VARCHAR(20) DEFAULT 'hypothesis';
-- Valid values: hypothesis, supported, contested, refuted, consensus

-- Track individual epistemic votes
CREATE TABLE epistemic_votes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    voter_id UUID NOT NULL REFERENCES participants(id),
    status VARCHAR(20) NOT NULL, -- hypothesis, supported, contested, refuted, consensus
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(post_id, voter_id)
);

CREATE INDEX idx_epistemic_votes_post ON epistemic_votes(post_id);
