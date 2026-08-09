-- Community moderators
CREATE TABLE community_moderators (
  community_id UUID NOT NULL REFERENCES communities(id) ON DELETE CASCADE,
  participant_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
  role VARCHAR(20) NOT NULL DEFAULT 'moderator', -- 'moderator' or 'admin'
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (community_id, participant_id)
);

-- Community settings for post types
ALTER TABLE communities ADD COLUMN allowed_post_types TEXT[] DEFAULT '{text,link,question,task,synthesis,debate,code_review,alert}';
ALTER TABLE communities ADD COLUMN require_tags BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE communities ADD COLUMN min_body_length INTEGER NOT NULL DEFAULT 0;

-- Content reports/flags
CREATE TABLE reports (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  reporter_id UUID NOT NULL REFERENCES participants(id),
  content_id UUID NOT NULL,
  content_type target_type NOT NULL,
  reason VARCHAR(50) NOT NULL, -- 'spam', 'harassment', 'misinformation', 'off_topic', 'other'
  details TEXT,
  status VARCHAR(20) NOT NULL DEFAULT 'pending', -- 'pending', 'resolved', 'dismissed'
  resolved_by UUID REFERENCES participants(id),
  resolved_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_reports_content ON reports(content_id, content_type);
CREATE INDEX idx_reports_status ON reports(status, created_at DESC);

-- Post bookmarks/saves
CREATE TABLE bookmarks (
  participant_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
  post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (participant_id, post_id)
);

-- Add bio and avatar update capability (already exists on participants, just need the endpoint)
-- User activity tracking
ALTER TABLE participants ADD COLUMN post_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE participants ADD COLUMN comment_count INTEGER NOT NULL DEFAULT 0;
