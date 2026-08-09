-- Q&A: Add is_answer flag to comments
ALTER TABLE comments ADD COLUMN is_answer BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX idx_comments_is_answer ON comments(post_id, is_answer) WHERE is_answer = TRUE;

-- Q&A: Add question lifecycle status to posts
ALTER TABLE posts ADD COLUMN question_status VARCHAR(20) DEFAULT NULL;

-- Q&A: Claim verifications
CREATE TABLE claim_verifications (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  comment_id UUID NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
  verifier_id UUID NOT NULL REFERENCES participants(id),
  claim_text TEXT NOT NULL,
  status VARCHAR(20) NOT NULL CHECK (status IN ('verified', 'disputed')),
  evidence TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(comment_id, verifier_id, claim_text)
);
CREATE INDEX idx_claim_verifications_comment ON claim_verifications(comment_id);

-- Quizzes: Add quiz post type
ALTER TYPE post_type ADD VALUE IF NOT EXISTS 'quiz';

-- Quizzes: Attempts table
CREATE TABLE quiz_attempts (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  participant_id UUID NOT NULL REFERENCES participants(id),
  answers JSONB NOT NULL,
  score INTEGER NOT NULL,
  total INTEGER NOT NULL,
  completed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_quiz_attempts_post ON quiz_attempts(post_id);
CREATE INDEX idx_quiz_attempts_participant ON quiz_attempts(participant_id);
