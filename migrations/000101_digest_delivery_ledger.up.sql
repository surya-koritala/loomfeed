CREATE TABLE digest_deliveries (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    recipient_id UUID NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
    cadence VARCHAR(16) NOT NULL,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    first_attempt_at TIMESTAMPTZ,
    claim_expires_at TIMESTAMPTZ,
    sent_at TIMESTAMPTZ,
    last_error TEXT,
    recipient_email TEXT,
    recipient_name TEXT,
    subject TEXT,
    html_body TEXT,
    plain_text TEXT,
    post_ids UUID[],
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT digest_deliveries_cadence_chk
        CHECK (cadence IN ('daily', 'weekly')),
    CONSTRAINT digest_deliveries_status_chk
        CHECK (status IN ('pending', 'sending', 'failed', 'sent', 'canceled')),
    CONSTRAINT digest_deliveries_period_chk
        CHECK (period_end > period_start),
    CONSTRAINT digest_deliveries_pending_payload_chk
        CHECK (status IN ('sent', 'canceled') OR (
            recipient_email IS NOT NULL AND recipient_name IS NOT NULL AND
            subject IS NOT NULL AND html_body IS NOT NULL AND plain_text IS NOT NULL AND
            post_ids IS NOT NULL
        )),
    CONSTRAINT digest_deliveries_recipient_period_uniq
        UNIQUE (recipient_id, cadence, period_start)
);

CREATE INDEX idx_digest_deliveries_retryable
    ON digest_deliveries(cadence, period_start, status, claim_expires_at)
    WHERE status IN ('pending', 'sending', 'failed');

-- Failed/in-flight messages temporarily retain their exact provider payload so
-- a retry with the same provider idempotency key also has identical content.
-- HTTP request contexts must not be able to read another user's pending email.
ALTER TABLE digest_deliveries ENABLE ROW LEVEL SECURITY;
ALTER TABLE digest_deliveries FORCE ROW LEVEL SECURITY;

CREATE POLICY digest_deliveries_service ON digest_deliveries FOR ALL TO app_service
    USING (TRUE)
    WITH CHECK (TRUE);
