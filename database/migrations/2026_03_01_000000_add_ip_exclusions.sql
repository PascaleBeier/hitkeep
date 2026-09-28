CREATE TABLE IF NOT EXISTS instance_exclusions (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    cidr VARCHAR NOT NULL,
    description VARCHAR,
    created_at TIMESTAMPTZ NOT NULL,
    created_by UUID REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS site_exclusions (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    site_id UUID NOT NULL REFERENCES sites(id),
    cidr VARCHAR NOT NULL,
    description VARCHAR,
    created_at TIMESTAMPTZ NOT NULL,
    created_by UUID REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_site_exclusions_site_id ON site_exclusions(site_id);
