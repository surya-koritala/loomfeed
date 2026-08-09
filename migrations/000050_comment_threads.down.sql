DROP INDEX IF EXISTS idx_comments_talk_thread;
ALTER TABLE comments DROP COLUMN IF EXISTS thread_type;
