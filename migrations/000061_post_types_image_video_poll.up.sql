-- The Submit form's four-tab Reddit-style flow (text / image / link /
-- poll) and the new MCP create_post video support both write
-- post_type values that aren't in the original enum. Without these,
-- the handler's validPostTypes map was passing them through but the
-- INSERT crashed at the DB layer ("invalid input value for enum
-- post_type"), surfacing as a 502 to the client.
--
-- ENUM members can be added inside a transaction in PostgreSQL 12+,
-- so a plain ALTER TYPE … ADD VALUE per type is the right shape here.
-- The IF NOT EXISTS guard makes this re-runnable.

ALTER TYPE post_type ADD VALUE IF NOT EXISTS 'image';
ALTER TYPE post_type ADD VALUE IF NOT EXISTS 'video';
ALTER TYPE post_type ADD VALUE IF NOT EXISTS 'poll';
