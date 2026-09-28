CREATE TABLE IF NOT EXISTS team_sso_configs (
    tenant_id UUID PRIMARY KEY REFERENCES tenants (id),
    provider_type VARCHAR NOT NULL DEFAULT 'oidc' CHECK (provider_type = 'oidc'),
    issuer_url VARCHAR NOT NULL,
    client_id VARCHAR NOT NULL,
    client_secret_encrypted VARCHAR NOT NULL,
    email_claim VARCHAR NOT NULL DEFAULT 'email',
    display_name_claim VARCHAR NOT NULL DEFAULT 'name',
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS team_sso_domains (
    tenant_id UUID NOT NULL REFERENCES team_sso_configs (tenant_id),
    domain VARCHAR NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, domain),
    UNIQUE (domain)
);

CREATE INDEX IF NOT EXISTS team_sso_domains_tenant_idx ON team_sso_domains (tenant_id);

CREATE TABLE IF NOT EXISTS sso_identities (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    tenant_id UUID NOT NULL REFERENCES tenants (id),
    user_id UUID NOT NULL REFERENCES users (id),
    issuer_url VARCHAR NOT NULL,
    subject VARCHAR NOT NULL,
    email VARCHAR NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, issuer_url, subject),
    UNIQUE (tenant_id, user_id)
);

CREATE INDEX IF NOT EXISTS sso_identities_tenant_idx ON sso_identities (tenant_id);
CREATE INDEX IF NOT EXISTS sso_identities_user_idx ON sso_identities (user_id);
