CREATE TEMP TABLE api_clients_backup AS
SELECT
    id,
    user_id,
    name,
    description,
    secret_hash,
    instance_role,
    expires_at,
    last_used_at,
    revoked_at,
    created_at,
    updated_at
FROM api_clients;

CREATE TEMP TABLE api_client_site_roles_backup AS
SELECT id, api_client_id, site_id, role, created_at
FROM api_client_site_roles;

DROP TABLE api_client_site_roles;
DROP TABLE api_clients;

CREATE TABLE api_clients (
    id            UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id       UUID REFERENCES users(id),
    tenant_id     UUID REFERENCES tenants(id),
    name          VARCHAR     NOT NULL,
    description   VARCHAR,
    secret_hash   VARCHAR     UNIQUE NOT NULL,
    instance_role VARCHAR     NOT NULL DEFAULT 'user',
    expires_at    TIMESTAMPTZ,
    last_used_at  TIMESTAMPTZ,
    revoked_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL
);

INSERT INTO api_clients (
    id,
    user_id,
    tenant_id,
    name,
    description,
    secret_hash,
    instance_role,
    expires_at,
    last_used_at,
    revoked_at,
    created_at,
    updated_at
)
SELECT
    id,
    user_id,
    NULL,
    name,
    description,
    secret_hash,
    instance_role,
    expires_at,
    last_used_at,
    revoked_at,
    created_at,
    updated_at
FROM api_clients_backup;

CREATE INDEX IF NOT EXISTS api_clients_user_id_idx ON api_clients (user_id);
CREATE INDEX IF NOT EXISTS api_clients_tenant_id_idx ON api_clients (tenant_id);
CREATE INDEX IF NOT EXISTS api_clients_secret_hash_idx ON api_clients (secret_hash);

CREATE TABLE api_client_site_roles (
    id            UUID PRIMARY KEY DEFAULT uuidv7(),
    api_client_id UUID        NOT NULL REFERENCES api_clients(id),
    site_id       UUID        NOT NULL REFERENCES sites(id),
    role          VARCHAR     NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL,
    UNIQUE (api_client_id, site_id)
);

INSERT INTO api_client_site_roles (id, api_client_id, site_id, role, created_at)
SELECT id, api_client_id, site_id, role, created_at
FROM api_client_site_roles_backup;

CREATE INDEX IF NOT EXISTS api_client_site_roles_client_id_idx ON api_client_site_roles (api_client_id);
CREATE INDEX IF NOT EXISTS api_client_site_roles_site_id_idx ON api_client_site_roles (site_id);

DROP TABLE api_clients_backup;
DROP TABLE api_client_site_roles_backup;
