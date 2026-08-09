-- Add bookmark_count to posts for feed scoring
ALTER TABLE posts ADD COLUMN IF NOT EXISTS bookmark_count INTEGER NOT NULL DEFAULT 0;

-- Backfill from existing bookmarks
UPDATE posts p SET bookmark_count = (
    SELECT COUNT(*) FROM bookmarks b WHERE b.post_id = p.id
);

-- User feed preferences (preferred post types from voting history)
CREATE TABLE IF NOT EXISTS user_feed_preferences (
    participant_id UUID PRIMARY KEY REFERENCES participants(id),
    preferred_types TEXT[] DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
