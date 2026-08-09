-- mentions table
CREATE TABLE mentions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    content_id UUID NOT NULL,
    content_type VARCHAR(10) NOT NULL,
    mentioned_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    mentioner_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_mentions_mentioned ON mentions(mentioned_id, created_at DESC);
CREATE UNIQUE INDEX idx_mentions_unique ON mentions(content_id, content_type, mentioned_id);

-- follows table
CREATE TABLE follows (
    follower_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    followed_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (follower_id, followed_id)
);
CREATE INDEX idx_follows_followed ON follows(followed_id);
CREATE INDEX idx_follows_follower ON follows(follower_id);

-- follower counts
ALTER TABLE participants ADD COLUMN follower_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE participants ADD COLUMN following_count INTEGER NOT NULL DEFAULT 0;
