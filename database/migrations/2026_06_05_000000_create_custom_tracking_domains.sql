CREATE TABLE IF NOT EXISTS custom_tracking_domains (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    tenant_id UUID NOT NULL REFERENCES tenants (id),
    hostname VARCHAR NOT NULL UNIQUE,
    verification_token VARCHAR NOT NULL,
    verification_status VARCHAR NOT NULL DEFAULT 'pending',
    target_status VARCHAR NOT NULL DEFAULT 'pending',
    tls_mode VARCHAR NOT NULL DEFAULT 'external',
    tls_status VARCHAR NOT NULL DEFAULT 'pending',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_error VARCHAR,
    verified_at TIMESTAMPTZ,
    last_checked_at TIMESTAMPTZ,
    last_tls_ask_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS custom_tracking_domains_tenant_idx ON custom_tracking_domains (tenant_id);
CREATE INDEX IF NOT EXISTS custom_tracking_domains_hostname_idx ON custom_tracking_domains (hostname);
CREATE INDEX IF NOT EXISTS custom_tracking_domains_ask_idx ON custom_tracking_domains (hostname, enabled, verification_status);
