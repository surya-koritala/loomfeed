-- Full-text search index on posts
ALTER TABLE posts ADD COLUMN search_vector tsvector;

CREATE INDEX idx_posts_search ON posts USING GIN (search_vector);

-- Trigger to auto-update search_vector on insert/update
CREATE OR REPLACE FUNCTION posts_search_update() RETURNS trigger AS $$
BEGIN
  NEW.search_vector := setweight(to_tsvector('english', COALESCE(NEW.title, '')), 'A') ||
                        setweight(to_tsvector('english', COALESCE(NEW.body, '')), 'B');
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tr_posts_search_update
  BEFORE INSERT OR UPDATE OF title, body ON posts
  FOR EACH ROW EXECUTE FUNCTION posts_search_update();

-- Backfill existing posts
UPDATE posts SET search_vector = setweight(to_tsvector('english', COALESCE(title, '')), 'A') ||
                                  setweight(to_tsvector('english', COALESCE(body, '')), 'B');

-- Notifications table
CREATE TABLE notifications (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  recipient_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
  type VARCHAR(50) NOT NULL,  -- 'comment_reply', 'post_comment', 'vote', 'mention', 'accepted_answer'
  actor_id UUID REFERENCES participants(id),
  post_id UUID REFERENCES posts(id) ON DELETE CASCADE,
  comment_id UUID REFERENCES comments(id) ON DELETE CASCADE,
  message TEXT NOT NULL,
  is_read BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_recipient ON notifications(recipient_id, is_read, created_at DESC);
