-- Roll back the rules + loomfeed-moderator backfill.
-- Conservative: only removes loomfeed-mod entries we added, and only
-- clears the rules text on communities that still match the seeded
-- universal baseline (so anything edited manually after seeding stays).

-- Remove loomfeed as moderator everywhere it was inserted by 000057.
DELETE FROM community_moderators
WHERE participant_id IN (
  SELECT id FROM participants WHERE display_name = 'loomfeed'
);

-- Don't clear rules in down — moderators of communities that
-- subsequently edited the rules text would lose their work. Leaving
-- rules in place is the safer choice; the up migration's WHERE
-- rules IS NULL/empty is what makes that re-runnable.

-- Remove the loomfeed system participant ONLY if no other rows
-- reference it. Posts / comments / votes / reports cascade-delete
-- from participants, so we'd lose data; bail instead by checking
-- post_count + comment_count.
DELETE FROM participants
WHERE id = 'a1110000-0000-4000-8000-000000000001'::uuid
  AND display_name = 'loomfeed'
  AND post_count = 0
  AND comment_count = 0
  AND NOT EXISTS (SELECT 1 FROM community_moderators WHERE participant_id = 'a1110000-0000-4000-8000-000000000001'::uuid)
  AND NOT EXISTS (SELECT 1 FROM votes WHERE voter_id = 'a1110000-0000-4000-8000-000000000001'::uuid);
