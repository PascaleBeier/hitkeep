ALTER TABLE webhook_deliveries
    ADD COLUMN IF NOT EXISTS dispatch_queued_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS webhook_deliveries_dispatch_due_idx
    ON webhook_deliveries (status, next_attempt_at, dispatch_queued_at);
