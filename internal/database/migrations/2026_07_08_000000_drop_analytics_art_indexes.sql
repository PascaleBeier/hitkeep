-- Drop the ART indexes from the append-only analytics tables.
--
-- DuckDB keeps every PRIMARY KEY / FOREIGN KEY / CREATE INDEX as an in-memory
-- ART index that grows with row count and is never freed at runtime. On the
-- hits table these indexes measured ~60% of the database file and ~0.9 GB of
-- resident memory per 9M rows, while dashboard queries never use them (zone
-- maps prune the scans). Index maintenance also dominated insert cost.
-- DuckDB cannot drop a PRIMARY KEY in place, so the tables are rebuilt.
-- Site-transfer copies no longer rely on INSERT OR REPLACE for these tables.

DROP INDEX IF EXISTS hits_site_id_timestamp_idx;
DROP INDEX IF EXISTS hits_qr_code_id_idx;
CREATE TABLE hits_rebuild (
    id              UUID        NOT NULL DEFAULT uuidv7(),
    site_id         UUID        NOT NULL,
    session_id      UUID        NOT NULL,
    page_id         UUID        NOT NULL,
    timestamp       TIMESTAMPTZ NOT NULL,
    path            VARCHAR     NOT NULL,
    hostname        VARCHAR,
    referrer        VARCHAR,
    user_agent      VARCHAR,
    viewport_width  INTEGER,
    viewport_height INTEGER,
    screen_width    INTEGER,
    screen_height   INTEGER,
    language        VARCHAR,
    is_unique       BOOLEAN,
    country_code    VARCHAR,
    utm_source      VARCHAR,
    utm_medium      VARCHAR,
    utm_campaign    VARCHAR,
    utm_term        VARCHAR,
    utm_content     VARCHAR,
    region          VARCHAR,
    city            VARCHAR,
    provider        VARCHAR,
    asn             INTEGER,
    asn_org         VARCHAR,
    qr_code_id      UUID
);
INSERT INTO hits_rebuild SELECT * FROM hits;
DROP TABLE hits;
ALTER TABLE hits_rebuild RENAME TO hits;
