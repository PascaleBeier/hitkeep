CREATE TABLE IF NOT EXISTS team_audit_log (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    tenant_id UUID NOT NULL REFERENCES tenants (id),
    actor_id UUID REFERENCES users (id),
    target_user_id UUID REFERENCES users (id),
    action VARCHAR NOT NULL,
    details VARCHAR NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS team_audit_log_tenant_created_idx ON team_audit_log (tenant_id, created_at DESC);
