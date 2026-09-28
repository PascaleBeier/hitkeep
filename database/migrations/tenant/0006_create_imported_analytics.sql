CREATE TABLE IF NOT EXISTS imported_traffic_daily (
    site_id        UUID NOT NULL,
    import_id      UUID NOT NULL,
    date           DATE NOT NULL,
    visitors       BIGINT NOT NULL DEFAULT 0,
    visits         BIGINT NOT NULL DEFAULT 0,
    pageviews      BIGINT NOT NULL DEFAULT 0,
    bounces        BIGINT NOT NULL DEFAULT 0,
    visit_duration BIGINT NOT NULL DEFAULT 0,
    source_file    VARCHAR NOT NULL
);
CREATE INDEX IF NOT EXISTS imported_traffic_daily_site_date_idx ON imported_traffic_daily (site_id, date);
CREATE INDEX IF NOT EXISTS imported_traffic_daily_import_idx ON imported_traffic_daily (import_id);

CREATE TABLE IF NOT EXISTS imported_dimension_daily (
    site_id        UUID NOT NULL,
    import_id      UUID NOT NULL,
    date           DATE NOT NULL,
    dimension      VARCHAR NOT NULL,
    name           VARCHAR NOT NULL,
    detail         VARCHAR,
    visitors       BIGINT NOT NULL DEFAULT 0,
    visits         BIGINT NOT NULL DEFAULT 0,
    pageviews      BIGINT NOT NULL DEFAULT 0,
    bounces        BIGINT NOT NULL DEFAULT 0,
    visit_duration BIGINT NOT NULL DEFAULT 0,
    events         BIGINT NOT NULL DEFAULT 0,
    entrances      BIGINT NOT NULL DEFAULT 0,
    exits          BIGINT NOT NULL DEFAULT 0,
    source_file    VARCHAR NOT NULL
);
CREATE INDEX IF NOT EXISTS imported_dimension_daily_site_dim_date_idx ON imported_dimension_daily (site_id, dimension, date);
CREATE INDEX IF NOT EXISTS imported_dimension_daily_import_idx ON imported_dimension_daily (import_id);

CREATE TABLE IF NOT EXISTS imported_event_daily (
    site_id     UUID NOT NULL,
    import_id   UUID NOT NULL,
    date        DATE NOT NULL,
    event_name  VARCHAR NOT NULL,
    path        VARCHAR,
    link_url    VARCHAR,
    visitors    BIGINT NOT NULL DEFAULT 0,
    events      BIGINT NOT NULL DEFAULT 0,
    source_file VARCHAR NOT NULL
);
CREATE INDEX IF NOT EXISTS imported_event_daily_site_event_date_idx ON imported_event_daily (site_id, event_name, date);
CREATE INDEX IF NOT EXISTS imported_event_daily_import_idx ON imported_event_daily (import_id);

CREATE TABLE IF NOT EXISTS imported_event_dimensions_daily (
    site_id     UUID NOT NULL,
    import_id   UUID NOT NULL,
    date        DATE NOT NULL,
    event_name  VARCHAR NOT NULL,
    dimension   VARCHAR NOT NULL,
    name        VARCHAR NOT NULL,
    detail      VARCHAR,
    visitors    BIGINT NOT NULL DEFAULT 0,
    events      BIGINT NOT NULL DEFAULT 0,
    source_file VARCHAR NOT NULL
);
CREATE INDEX IF NOT EXISTS imported_event_dimensions_daily_site_dim_date_idx ON imported_event_dimensions_daily (site_id, event_name, dimension, date);
CREATE INDEX IF NOT EXISTS imported_event_dimensions_daily_import_idx ON imported_event_dimensions_daily (import_id);

CREATE TABLE IF NOT EXISTS imported_event_properties_daily (
    site_id        UUID NOT NULL,
    import_id      UUID NOT NULL,
    date           DATE NOT NULL,
    event_name     VARCHAR NOT NULL DEFAULT '',
    property_key   VARCHAR NOT NULL,
    property_value VARCHAR NOT NULL,
    visitors       BIGINT NOT NULL DEFAULT 0,
    events         BIGINT NOT NULL DEFAULT 0,
    source_file    VARCHAR NOT NULL
);
CREATE INDEX IF NOT EXISTS imported_event_properties_daily_site_event_date_idx ON imported_event_properties_daily (site_id, event_name, date);
CREATE INDEX IF NOT EXISTS imported_event_properties_daily_key_idx ON imported_event_properties_daily (site_id, property_key, date);
CREATE INDEX IF NOT EXISTS imported_event_properties_daily_import_idx ON imported_event_properties_daily (import_id);
