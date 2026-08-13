ALTER TABLE webhook_deliveries
    ADD COLUMN event_id UUID;

-- Historical rows predate event envelopes. Their delivery UUID is a stable,
-- unique identifier that preserves a useful audit reference during upgrade.
UPDATE webhook_deliveries
SET event_id = id
WHERE event_id IS NULL;

ALTER TABLE webhook_deliveries
    ALTER COLUMN event_id SET DEFAULT uuid_generate_v4();

ALTER TABLE webhook_deliveries
    ALTER COLUMN event_id SET NOT NULL;

-- Keep the default during rolling upgrades. Older API replicas omit event_id;
-- they receive a per-delivery UUID until replaced, while current replicas
-- supply the logical event UUID shared by fan-out deliveries.

CREATE INDEX idx_webhook_deliveries_event_id
    ON webhook_deliveries(event_id);
