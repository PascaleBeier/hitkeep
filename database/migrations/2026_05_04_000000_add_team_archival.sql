CREATE TABLE IF NOT EXISTS tenant_archives (
    tenant_id UUID PRIMARY KEY REFERENCES tenants (id),
    archived_at TIMESTAMPTZ NOT NULL,
    archived_by UUID
);

CREATE INDEX IF NOT EXISTS tenant_archives_archived_at_idx ON tenant_archives (archived_at DESC);
