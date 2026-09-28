CREATE TABLE IF NOT EXISTS cloud_lifecycle_messages (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants (id),
    user_id UUID NOT NULL REFERENCES users (id),
    kind VARCHAR NOT NULL,
    status VARCHAR NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    processing_error VARCHAR,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, user_id, kind)
);

CREATE INDEX IF NOT EXISTS cloud_lifecycle_messages_tenant_kind_idx ON cloud_lifecycle_messages (tenant_id, kind);
CREATE INDEX IF NOT EXISTS cloud_lifecycle_messages_status_idx ON cloud_lifecycle_messages (status);
