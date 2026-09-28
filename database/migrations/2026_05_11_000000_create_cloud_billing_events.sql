CREATE TABLE IF NOT EXISTS cloud_billing_events (
    stripe_event_id VARCHAR PRIMARY KEY,
    tenant_id UUID REFERENCES tenants (id),
    event_type VARCHAR NOT NULL,
    livemode BOOLEAN NOT NULL DEFAULT FALSE,
    payload TEXT,
    processing_status VARCHAR NOT NULL,
    processing_error TEXT,
    processed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS cloud_billing_events_tenant_created_idx
ON cloud_billing_events (tenant_id, created_at DESC);
