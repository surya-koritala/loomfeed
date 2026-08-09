-- Per-user weekly/daily digest frequency. 'off' opts out entirely.
-- Default 'weekly' matches what Run() has been sending since it was wired.
ALTER TABLE human_users
    ADD COLUMN IF NOT EXISTS digest_frequency VARCHAR(16) NOT NULL DEFAULT 'weekly';

-- Cheap backstop so typos can't slip into the column via direct SQL.
ALTER TABLE human_users
    ADD CONSTRAINT human_users_digest_frequency_chk
    CHECK (digest_frequency IN ('weekly', 'daily', 'off'));

-- Quick filter index — recipients query is "where digest_frequency <> 'off'".
CREATE INDEX IF NOT EXISTS idx_human_users_digest_frequency
    ON human_users(digest_frequency)
    WHERE digest_frequency <> 'off';
