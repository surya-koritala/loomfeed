-- Backfill poll rows for posts created before the poll-persistence fix
-- (#151). The old Submit form stored a poll's options only in
-- posts.metadata->'poll' (a JSONB blob) and never created rows in the
-- polls / poll_options tables, so those posts render with no options and
-- can't be voted on. This migration materializes the real poll for every
-- such post so they show + become votable.
--
-- Idempotent: the NOT EXISTS guard means re-running is a no-op, and it
-- only touches posts that have >=2 options in metadata and no poll yet.

WITH candidates AS (
    SELECT
        p.id AS post_id,
        p.metadata->'poll'->'options' AS options,
        NULLIF(p.metadata->'poll'->>'deadline', '') AS deadline
    FROM posts p
    WHERE jsonb_typeof(p.metadata->'poll'->'options') = 'array'
      AND jsonb_array_length(p.metadata->'poll'->'options') >= 2
      AND NOT EXISTS (SELECT 1 FROM polls pl WHERE pl.post_id = p.id)
),
ins_poll AS (
    INSERT INTO polls (post_id, deadline)
    SELECT
        post_id,
        -- Old metadata deadlines may be RFC3339 or a bare datetime-local
        -- value; both start with YYYY-MM-DDT and cast cleanly. Anything
        -- else (or absent) becomes an open-ended poll.
        CASE WHEN deadline ~ '^\d{4}-\d{2}-\d{2}T' THEN deadline::timestamptz ELSE NULL END
    FROM candidates
    RETURNING id AS poll_id, post_id
)
INSERT INTO poll_options (poll_id, text, sort_order)
SELECT ip.poll_id, opt.value, (opt.ord - 1)::int
FROM ins_poll ip
JOIN candidates c ON c.post_id = ip.post_id
CROSS JOIN LATERAL jsonb_array_elements_text(c.options) WITH ORDINALITY AS opt(value, ord)
WHERE trim(opt.value) <> '';
