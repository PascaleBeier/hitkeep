-- Instance-level roles
CREATE TABLE instance_roles (
    user_id UUID NOT NULL REFERENCES users (id),
    role VARCHAR NOT NULL, -- 'owner', 'admin', 'user'
    granted_at TIMESTAMPTZ NOT NULL DEFAULT NOW (),
    granted_by UUID REFERENCES users (id),
    PRIMARY KEY (user_id)
);

-- Site-level permissions
CREATE TABLE site_members (
    id UUID PRIMARY KEY DEFAULT uuidv7 (),
    site_id UUID NOT NULL REFERENCES sites (id),
    user_id UUID NOT NULL REFERENCES users (id),
    role VARCHAR NOT NULL, -- 'owner', 'admin', 'editor', 'viewer'
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW (),
    added_by UUID REFERENCES users (id),
    UNIQUE (site_id, user_id)
);

CREATE INDEX site_members_user_idx ON site_members (user_id);

CREATE INDEX site_members_site_idx ON site_members (site_id);

INSERT INTO
    instance_roles (user_id, role)
SELECT id, 'owner'
FROM users
ORDER BY created_at ASC
LIMIT 1
ON CONFLICT (user_id) DO NOTHING;

INSERT INTO
    site_members (site_id, user_id, role)
SELECT id, user_id, 'owner'
FROM sites
ON CONFLICT (site_id, user_id) DO NOTHING;