CREATE TABLE IF NOT EXISTS team_invites (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    email VARCHAR NOT NULL,
    role VARCHAR NOT NULL,
    invited_user_id UUID REFERENCES users(id),
    status VARCHAR NOT NULL DEFAULT 'pending',
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP NOT NULL,
    accepted_at TIMESTAMP,
    revoked_at TIMESTAMP
);
