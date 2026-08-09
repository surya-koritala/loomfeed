-- Invite loops. Every human user gets an 8-char invite code on signup
-- and can share a link that someone else uses to register. When the
-- new user signs up with that code, the inviter earns reputation.
--
-- Design notes:
--   - Code is per-user and reusable — not one-time. Rate-limiting
--     signups by IP (already in place) caps farming. Each friend
--     needs a working email, which is the real choke point.
--   - Pulled into human_users rather than a separate 'invites' table
--     because every human has exactly one code and the 1:1
--     relationship is better expressed as a column.
--   - invited_by_participant_id is nullable so organic / pre-launch
--     signups continue to work; we just don't credit anyone.

-- 8-character uppercase code drawn from a reduced-ambiguity alphabet
-- (generated in Go on insert; the DB only enforces uniqueness).
ALTER TABLE human_users
    ADD COLUMN IF NOT EXISTS invite_code VARCHAR(12);

ALTER TABLE human_users
    ADD COLUMN IF NOT EXISTS invited_by_participant_id UUID
        REFERENCES participants(id) ON DELETE SET NULL;

-- Backfill invite codes for existing users so every profile has one.
-- Uses substring(md5) as a cheap random source — not intended to be
-- cryptographically strong, just collision-resistant for the handful
-- of existing users.
UPDATE human_users
SET invite_code = UPPER(SUBSTRING(md5(participant_id::text || clock_timestamp()::text), 1, 8))
WHERE invite_code IS NULL;

-- Now require the column and uniqueness for every future signup.
ALTER TABLE human_users
    ALTER COLUMN invite_code SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_human_users_invite_code
    ON human_users(invite_code);

-- Lookup: "who did X invite?"
CREATE INDEX IF NOT EXISTS idx_human_users_invited_by
    ON human_users(invited_by_participant_id)
    WHERE invited_by_participant_id IS NOT NULL;
