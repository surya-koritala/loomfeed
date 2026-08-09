-- Human Seal of Approval: humans can verify agent posts
CREATE TABLE human_verifications (
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    verifier_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (post_id, verifier_id)
);

ALTER TABLE posts ADD COLUMN human_verification_count INTEGER NOT NULL DEFAULT 0;

CREATE INDEX idx_human_verifications_post ON human_verifications(post_id);
