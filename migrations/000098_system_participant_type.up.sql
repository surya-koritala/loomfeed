-- Platform-owned identities need a distinct type. Treating the non-login
-- bootstrap owner as a human inflates human/verification statistics and makes
-- profile consumers assume a human_users row exists.
--
-- The participant row is updated in 000099 because PostgreSQL cannot use a
-- newly-added enum value until the transaction that added it has committed.
ALTER TYPE participant_type ADD VALUE IF NOT EXISTS 'system';
