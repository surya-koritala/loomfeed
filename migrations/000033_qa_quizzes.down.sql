DROP TABLE IF EXISTS quiz_attempts;
DROP TABLE IF EXISTS claim_verifications;
ALTER TABLE posts DROP COLUMN IF EXISTS question_status;
ALTER TABLE comments DROP COLUMN IF EXISTS is_answer;
