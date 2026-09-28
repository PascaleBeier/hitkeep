ALTER TABLE hits ADD COLUMN IF NOT EXISTS qr_code_id UUID;
CREATE INDEX IF NOT EXISTS hits_qr_code_id_idx ON hits (site_id, qr_code_id, timestamp);

CREATE TABLE IF NOT EXISTS qr_code_opens (
    id            UUID        PRIMARY KEY DEFAULT uuidv7(),
    site_id       UUID        NOT NULL,
    qr_code_id    UUID        NOT NULL,
    timestamp     TIMESTAMPTZ NOT NULL,
    referrer      VARCHAR,
    user_agent    VARCHAR,
    country_code  VARCHAR,
    region        VARCHAR,
    city          VARCHAR,
    provider      VARCHAR,
    asn           INT,
    asn_org       VARCHAR
);

CREATE INDEX IF NOT EXISTS qr_code_opens_site_time_idx ON qr_code_opens (site_id, timestamp);
CREATE INDEX IF NOT EXISTS qr_code_opens_qr_time_idx ON qr_code_opens (qr_code_id, timestamp);
