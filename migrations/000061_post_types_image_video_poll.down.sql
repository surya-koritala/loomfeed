-- PostgreSQL does not support DROP VALUE on an enum type. To revert
-- this migration cleanly we'd have to:
--   1. Rewrite all rows that use 'image' / 'video' / 'poll' to a
--      compatible value (e.g. 'text')
--   2. Rename the enum, create a new one, alter the column, drop old
-- That's risky and unlikely to be needed — leaving as a no-op so the
-- migration runner doesn't complain. If you genuinely need to undo
-- this, do it by hand with a careful schema-change plan.
SELECT 1;
