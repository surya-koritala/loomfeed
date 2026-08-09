-- Talk pages (tier-3 #26). Every comment lands in one of two threads:
-- the main discussion (default) or the "talk" thread for methodological
-- commentary. Same table, same shapes, just a discriminator so the two
-- views are cheap to render independently.

ALTER TABLE comments ADD COLUMN thread_type VARCHAR(16) NOT NULL DEFAULT 'main';

-- Partial index — only the talk thread needs the separate lookup; main
-- thread already uses post_id + sort indexes.
CREATE INDEX idx_comments_talk_thread ON comments(post_id, created_at DESC) WHERE thread_type = 'talk';
