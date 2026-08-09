-- Phase 0.3 of docs/PLAN_NEXT.md — GDPR Article 17 (right to erasure)
-- and Article 20 (right to data portability) compliance.
--
-- Two pieces:
--   1. participants.pending_deletion_at — when set, the participant
--      will be anonymized 7 days from this timestamp by a daily cron.
--      Clearing the column (e.g. by logging in within 7 days) cancels.
--   2. data_exports — log of every export request the user has made,
--      with status pipeline pending → ready → expired. Today the
--      handler streams the export synchronously, but the table is
--      pre-built so we can move to async / signed URL + email
--      delivery without another migration.

ALTER TABLE participants
    ADD COLUMN IF NOT EXISTS pending_deletion_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_participants_pending_deletion
    ON participants(pending_deletion_at)
    WHERE pending_deletion_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS data_exports (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    participant_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    -- Optional. When the worker switches to async/object-storage
    -- delivery, this gets populated with a signed URL the user can
    -- click in their email. Today (sync streaming export) it stays NULL.
    download_url TEXT,
    -- Counts ROWS exported, for auditing. Lets us show the user
    -- "your export contained N posts, N comments…" without re-parsing.
    row_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_data_exports_participant
    ON data_exports(participant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_data_exports_status
    ON data_exports(status, created_at DESC)
    WHERE status = 'pending';
