CREATE TEMP TABLE api_client_site_roles_backup AS
SELECT id, api_client_id, site_id, role, created_at
FROM api_client_site_roles;

DROP TABLE api_client_site_roles;

CREATE TABLE api_client_site_roles (
    id            UUID PRIMARY KEY DEFAULT uuidv7(),
    api_client_id UUID        NOT NULL,
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

DROP TABLE api_client_site_roles_backup;
