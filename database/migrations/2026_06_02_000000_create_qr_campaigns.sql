CREATE TABLE IF NOT EXISTS qr_codes (
    id                  UUID PRIMARY KEY DEFAULT uuidv7(),
    site_id             UUID        NOT NULL REFERENCES sites(id),
    created_by          UUID        REFERENCES users(id),
    name                VARCHAR     NOT NULL,
    destination_url     VARCHAR     NOT NULL,
    utm_source          VARCHAR,
    utm_medium          VARCHAR,
    utm_campaign        VARCHAR,
    utm_term            VARCHAR,
    utm_content         VARCHAR,
    custom_params_json  JSON,
    style_json          JSON,
    token               VARCHAR     UNIQUE NOT NULL,
    token_hash          VARCHAR     UNIQUE NOT NULL,
    token_hint          VARCHAR     NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL,
    archived_at         TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS qr_codes_site_id_idx ON qr_codes (site_id);
CREATE INDEX IF NOT EXISTS qr_codes_token_hash_idx ON qr_codes (token_hash);

CREATE TABLE IF NOT EXISTS qr_code_assets (
    qr_code_id    UUID PRIMARY KEY REFERENCES qr_codes(id),
    site_id       UUID        NOT NULL REFERENCES sites(id),
    filename      VARCHAR     NOT NULL,
    content_type  VARCHAR     NOT NULL,
    byte_size     BIGINT      NOT NULL,
    width         INTEGER,
    height        INTEGER,
    checksum      VARCHAR     NOT NULL,
    storage_key   VARCHAR     NOT NULL DEFAULT '',
    data          BLOB,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS qr_code_assets_site_id_idx ON qr_code_assets (site_id);

CREATE TABLE IF NOT EXISTS qr_code_share_links (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    site_id     UUID        NOT NULL REFERENCES sites(id),
    qr_code_id  UUID        NOT NULL REFERENCES qr_codes(id),
    token_hash  VARCHAR     UNIQUE NOT NULL,
    token_hint  VARCHAR     NOT NULL,
    created_by  UUID        REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS qr_code_share_links_site_id_idx ON qr_code_share_links (site_id);
CREATE INDEX IF NOT EXISTS qr_code_share_links_qr_code_id_idx ON qr_code_share_links (qr_code_id);
CREATE INDEX IF NOT EXISTS qr_code_share_links_token_hash_idx ON qr_code_share_links (token_hash);

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
