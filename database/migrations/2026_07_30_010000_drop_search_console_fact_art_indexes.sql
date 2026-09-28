-- Search Console facts are replaced in bounded date slices and queried through
-- aggregations. ART indexes do not accelerate those scans, remain resident in
-- memory once touched, and make large history refreshes unsafe under bounded
-- DuckDB memory. Rebuild the table as append-heavy analytics storage.

DROP INDEX IF EXISTS idx_search_console_facts_site_date;
DROP INDEX IF EXISTS idx_search_console_facts_site_page;
DROP INDEX IF EXISTS idx_search_console_facts_site_country_device;
CREATE TABLE search_console_facts_rebuild (
    site_id          UUID        NOT NULL,
    property_uri     VARCHAR     NOT NULL,
    date             DATE        NOT NULL,
    query            VARCHAR     NOT NULL DEFAULT '',
    page             VARCHAR     NOT NULL DEFAULT '',
    country          VARCHAR     NOT NULL DEFAULT '',
    device           VARCHAR     NOT NULL DEFAULT '',
    clicks           BIGINT      NOT NULL DEFAULT 0,
    impressions      BIGINT      NOT NULL DEFAULT 0,
    ctr              DOUBLE      NOT NULL DEFAULT 0,
    position         DOUBLE      NOT NULL DEFAULT 0,
    aggregation_type VARCHAR     NOT NULL DEFAULT '',
    data_state       VARCHAR     NOT NULL DEFAULT 'final',
    imported_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO search_console_facts_rebuild SELECT * FROM search_console_facts;
DROP TABLE search_console_facts;
ALTER TABLE search_console_facts_rebuild RENAME TO search_console_facts;
