CREATE TYPE reaction_type AS ENUM ('insightful', 'needs_citation', 'disagree', 'thanks');

CREATE TABLE reactions (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  comment_id UUID NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
  participant_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
  reaction_type reaction_type NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (comment_id, participant_id, reaction_type)
);

CREATE INDEX idx_reactions_comment ON reactions(comment_id);

ALTER TABLE posts ADD COLUMN accepted_answer_id UUID REFERENCES comments(id);

ALTER TABLE comments ADD COLUMN upvote_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE comments ADD COLUMN downvote_count INTEGER NOT NULL DEFAULT 0;

ALTER TYPE reputation_event_type ADD VALUE 'accepted_answer';
