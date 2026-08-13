-- PostgreSQL has no DROP VALUE for enums. The companion migration moves the
-- bootstrap participant back to `human`; leaving the unused value is safe.
SELECT 1;
