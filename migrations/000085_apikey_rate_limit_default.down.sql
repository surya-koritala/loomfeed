-- Revert the per-key request ceiling to the original 60/min default.
ALTER TABLE api_keys ALTER COLUMN rate_limit SET DEFAULT 60;
UPDATE api_keys SET rate_limit = 60 WHERE rate_limit = 300;
