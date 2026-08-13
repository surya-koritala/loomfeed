DROP INDEX IF EXISTS idx_webhook_deliveries_event_id;

ALTER TABLE webhook_deliveries
    DROP COLUMN IF EXISTS event_id;
