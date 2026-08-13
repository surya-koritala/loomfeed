ALTER TABLE webhooks
    ADD COLUMN delivery_claim_token UUID,
    ADD COLUMN delivery_claim_expires_at TIMESTAMPTZ;

CREATE TABLE webhook_outbox_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE webhook_delivery_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_id UUID NOT NULL REFERENCES webhook_outbox_events(id) ON DELETE CASCADE,
    webhook_id UUID NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claim_token UUID,
    claim_expires_at TIMESTAMPTZ,
    last_attempt_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT webhook_delivery_jobs_status_chk
        CHECK (status IN ('pending', 'processing', 'retry', 'succeeded', 'dead', 'canceled')),
    CONSTRAINT webhook_delivery_jobs_attempt_count_chk
        CHECK (attempt_count >= 0),
    CONSTRAINT webhook_delivery_jobs_event_webhook_uniq
        UNIQUE (event_id, webhook_id)
);

CREATE INDEX idx_webhook_delivery_jobs_claimable
    ON webhook_delivery_jobs(next_attempt_at, created_at)
    WHERE status IN ('pending', 'retry', 'processing');
CREATE INDEX idx_webhook_delivery_jobs_webhook
    ON webhook_delivery_jobs(webhook_id, created_at DESC);

ALTER TABLE webhook_deliveries
    ADD COLUMN job_id UUID REFERENCES webhook_delivery_jobs(id) ON DELETE SET NULL,
    ADD COLUMN attempt_number INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN terminal BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX idx_webhook_deliveries_job
    ON webhook_deliveries(job_id, attempt_number);

-- Request transactions may append outbox rows but cannot inspect them. Only
-- context-free background workers can claim/read/update queued payloads.
ALTER TABLE webhook_outbox_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE webhook_outbox_events FORCE ROW LEVEL SECURITY;
CREATE POLICY webhook_outbox_insert ON webhook_outbox_events FOR INSERT
    WITH CHECK (TRUE);
CREATE POLICY webhook_outbox_service ON webhook_outbox_events FOR ALL TO app_service
    USING (current_user = 'app_service')
    WITH CHECK (current_user = 'app_service');
CREATE POLICY webhook_outbox_owner_select ON webhook_outbox_events FOR SELECT TO app_user
    USING (EXISTS (
        SELECT 1
        FROM webhook_delivery_jobs j
        JOIN webhooks w ON w.id = j.webhook_id
        WHERE j.event_id = webhook_outbox_events.id
          AND w.participant_id = NULLIF(current_setting('app.current_user_id', true), '')::uuid
    ));

ALTER TABLE webhook_delivery_jobs ENABLE ROW LEVEL SECURITY;
ALTER TABLE webhook_delivery_jobs FORCE ROW LEVEL SECURITY;
CREATE POLICY webhook_jobs_insert ON webhook_delivery_jobs FOR INSERT
    WITH CHECK (TRUE);
CREATE POLICY webhook_jobs_service ON webhook_delivery_jobs FOR ALL TO app_service
    USING (current_user = 'app_service')
    WITH CHECK (current_user = 'app_service');
CREATE POLICY webhook_jobs_owner_select ON webhook_delivery_jobs FOR SELECT TO app_user
    USING (EXISTS (
        SELECT 1 FROM webhooks w
        WHERE w.id = webhook_delivery_jobs.webhook_id
          AND w.participant_id = NULLIF(current_setting('app.current_user_id', true), '')::uuid
    ));
