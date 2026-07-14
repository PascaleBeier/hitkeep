CREATE TEMP TABLE qr_code_assets_backup AS
SELECT qr_code_id, site_id, filename, content_type, byte_size, width, height,
       checksum, storage_key, data, created_at, updated_at
FROM qr_code_assets;

CREATE TEMP TABLE qr_code_share_links_backup AS
SELECT id, site_id, qr_code_id, token_hash, token_hint, created_by, created_at, revoked_at
FROM qr_code_share_links;

DROP TABLE qr_code_assets;
DROP TABLE qr_code_share_links;

CREATE TABLE qr_code_assets (
    qr_code_id    UUID PRIMARY KEY,
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

CREATE INDEX qr_code_assets_site_id_idx ON qr_code_assets (site_id);

CREATE TABLE qr_code_share_links (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    site_id     UUID        NOT NULL REFERENCES sites(id),
    qr_code_id  UUID        NOT NULL,
    token_hash  VARCHAR     UNIQUE NOT NULL,
    token_hint  VARCHAR     NOT NULL,
    created_by  UUID        REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ
);

CREATE INDEX qr_code_share_links_site_id_idx ON qr_code_share_links (site_id);
CREATE INDEX qr_code_share_links_qr_code_id_idx ON qr_code_share_links (qr_code_id);
CREATE INDEX qr_code_share_links_token_hash_idx ON qr_code_share_links (token_hash);

INSERT INTO qr_code_assets (
    qr_code_id, site_id, filename, content_type, byte_size, width, height,
    checksum, storage_key, data, created_at, updated_at
)
SELECT qr_code_id, site_id, filename, content_type, byte_size, width, height,
       checksum, storage_key, data, created_at, updated_at
FROM qr_code_assets_backup;

INSERT INTO qr_code_share_links (
    id, site_id, qr_code_id, token_hash, token_hint, created_by, created_at, revoked_at
)
SELECT id, site_id, qr_code_id, token_hash, token_hint, created_by, created_at, revoked_at
FROM qr_code_share_links_backup;

DROP TABLE qr_code_assets_backup;
DROP TABLE qr_code_share_links_backup;
