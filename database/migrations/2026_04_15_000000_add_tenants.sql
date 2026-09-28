CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    name VARCHAR NOT NULL,
    is_default BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tenant_members (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    tenant_id UUID NOT NULL REFERENCES tenants (id),
    user_id UUID NOT NULL REFERENCES users (id),
    role VARCHAR NOT NULL, -- 'owner', 'admin', 'member'
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    added_by UUID REFERENCES users (id),
    UNIQUE (tenant_id, user_id)
);

CREATE INDEX IF NOT EXISTS tenant_members_user_idx ON tenant_members (user_id);
CREATE INDEX IF NOT EXISTS tenant_members_tenant_idx ON tenant_members (tenant_id);

CREATE TABLE IF NOT EXISTS site_tenants (
    site_id UUID PRIMARY KEY REFERENCES sites (id),
    tenant_id UUID NOT NULL REFERENCES tenants (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (site_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS site_tenants_tenant_idx ON site_tenants (tenant_id);

INSERT INTO
    tenants (id, name, is_default)
SELECT
    uuidv7(),
    'Default Tenant',
    TRUE
WHERE NOT EXISTS (
    SELECT 1
    FROM tenants
    WHERE is_default = TRUE
);

INSERT INTO
    site_tenants (site_id, tenant_id)
SELECT
    s.id,
    (
        SELECT id
        FROM tenants
        WHERE is_default = TRUE
    ) AS tenant_id
FROM sites s
LEFT JOIN site_tenants st ON st.site_id = s.id
WHERE st.site_id IS NULL;

INSERT INTO
    tenant_members (tenant_id, user_id, role)
SELECT
    (
        SELECT id
        FROM tenants
        WHERE is_default = TRUE
    ) AS tenant_id,
    u.id AS user_id,
    CASE COALESCE(ir.role, 'user')
        WHEN 'owner' THEN 'owner'
        WHEN 'admin' THEN 'admin'
        ELSE 'member'
    END AS role
FROM users u
LEFT JOIN instance_roles ir ON ir.user_id = u.id
ON CONFLICT (tenant_id, user_id) DO NOTHING;
