DROP INDEX IF EXISTS idx_webhook_deliveries_job;

ALTER TABLE webhook_deliveries
    DROP COLUMN IF EXISTS terminal,
    DROP COLUMN IF EXISTS attempt_number,
    DROP COLUMN IF EXISTS job_id;

-- The outbox owner policy joins delivery jobs, so remove that cross-table
-- dependency before dropping the referenced table.
DROP POLICY IF EXISTS webhook_outbox_owner_select ON webhook_outbox_events;

DROP TABLE IF EXISTS webhook_delivery_jobs;
DROP TABLE IF EXISTS webhook_outbox_events;

ALTER TABLE webhooks
    DROP COLUMN IF EXISTS delivery_claim_expires_at,
    DROP COLUMN IF EXISTS delivery_claim_token;
