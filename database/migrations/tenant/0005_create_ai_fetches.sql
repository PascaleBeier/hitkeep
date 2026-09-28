-- Tenant analytics databases also include the local sites table, so ai_fetches
-- keeps the same foreign-key constraint as the shared analytics schema.
CREATE TABLE IF NOT EXISTS ai_fetches (
    id               UUID PRIMARY KEY DEFAULT uuidv7(),
    site_id          UUID        NOT NULL REFERENCES sites(id),
    timestamp        TIMESTAMPTZ NOT NULL,
    assistant_name   VARCHAR     NOT NULL,
    assistant_family VARCHAR     NOT NULL,
    path             VARCHAR     NOT NULL,
    hostname         VARCHAR,
    status_code      INTEGER     NOT NULL,
    content_type     VARCHAR,
    resource_type    VARCHAR     NOT NULL,
    response_ms      INTEGER,
    bytes_served     BIGINT,
    user_agent       VARCHAR
);
CREATE INDEX IF NOT EXISTS ai_fetches_site_id_timestamp_idx ON ai_fetches (site_id, timestamp);
