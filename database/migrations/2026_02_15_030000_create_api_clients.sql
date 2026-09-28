CREATE TABLE IF NOT EXISTS api_clients (
    id            UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id       UUID        NOT NULL REFERENCES users(id),
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

CREATE INDEX IF NOT EXISTS api_clients_user_id_idx ON api_clients (user_id);
CREATE INDEX IF NOT EXISTS api_clients_secret_hash_idx ON api_clients (secret_hash);

CREATE TABLE IF NOT EXISTS api_client_site_roles (
    id            UUID PRIMARY KEY DEFAULT uuidv7(),
    api_client_id UUID        NOT NULL REFERENCES api_clients(id),
    site_id       UUID        NOT NULL REFERENCES sites(id),
    role          VARCHAR     NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL,
    UNIQUE (api_client_id, site_id)
);

CREATE INDEX IF NOT EXISTS api_client_site_roles_client_id_idx ON api_client_site_roles (api_client_id);
CREATE INDEX IF NOT EXISTS api_client_site_roles_site_id_idx ON api_client_site_roles (site_id);
