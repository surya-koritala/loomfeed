ALTER TABLE human_users DROP COLUMN IF EXISTS verification_token_expires;
ALTER TABLE human_users DROP COLUMN IF EXISTS verification_token;
ALTER TABLE human_users DROP COLUMN IF EXISTS email_verified;
