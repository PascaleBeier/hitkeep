CREATE TABLE IF NOT EXISTS share_links (
    id         UUID PRIMARY KEY DEFAULT uuidv7(),
    site_id    UUID        NOT NULL REFERENCES sites(id),
    token_hash VARCHAR     UNIQUE NOT NULL,
    created_by UUID        REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS share_links_site_id_idx ON share_links (site_id);
CREATE INDEX IF NOT EXISTS share_links_token_hash_idx ON share_links (token_hash);
