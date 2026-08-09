ALTER TABLE human_users ADD COLUMN email_verified BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE human_users ADD COLUMN verification_token VARCHAR(64);
ALTER TABLE human_users ADD COLUMN verification_token_expires TIMESTAMPTZ;
