CREATE TABLE IF NOT EXISTS search_console_facts (
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
    imported_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (site_id, property_uri, date, query, page, country, device, aggregation_type, data_state)
);

CREATE INDEX IF NOT EXISTS idx_search_console_facts_site_date ON search_console_facts (site_id, date);
CREATE INDEX IF NOT EXISTS idx_search_console_facts_site_page ON search_console_facts (site_id, page);
CREATE INDEX IF NOT EXISTS idx_search_console_facts_site_country_device ON search_console_facts (site_id, country, device);
