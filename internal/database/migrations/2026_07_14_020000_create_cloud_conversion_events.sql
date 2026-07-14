CREATE TABLE IF NOT EXISTS cloud_conversion_events (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    event_name VARCHAR NOT NULL,
    plan_code VARCHAR NOT NULL,
    billing_interval VARCHAR NOT NULL,
    dedupe_key VARCHAR NOT NULL UNIQUE,
    occurred_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_cloud_conversion_events_tenant_time
    ON cloud_conversion_events(tenant_id, occurred_at);

CREATE INDEX IF NOT EXISTS idx_cloud_conversion_events_name_time
    ON cloud_conversion_events(event_name, occurred_at);
