ALTER TABLE sites
ADD COLUMN IF NOT EXISTS data_retention_days INTEGER DEFAULT 365;

CREATE TABLE IF NOT EXISTS events (
    id UUID PRIMARY KEY DEFAULT uuidv7 (),
    site_id UUID NOT NULL REFERENCES sites (id),
    session_id UUID NOT NULL,
    name VARCHAR NOT NULL,
    properties JSON,
    timestamp TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS events_site_id_timestamp_idx ON events (site_id, timestamp);

CREATE TABLE IF NOT EXISTS goals (
    id UUID PRIMARY KEY DEFAULT uuidv7 (),
    site_id UUID NOT NULL REFERENCES sites (id),
    name VARCHAR NOT NULL,
    type VARCHAR NOT NULL CHECK (type IN ('event', 'path')),
    value VARCHAR NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS goals_site_id_idx ON goals (site_id);

CREATE TABLE IF NOT EXISTS funnels (
    id UUID PRIMARY KEY DEFAULT uuidv7 (),
    site_id UUID NOT NULL REFERENCES sites (id),
    name VARCHAR NOT NULL,
    steps JSON NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS funnels_site_id_idx ON funnels (site_id);