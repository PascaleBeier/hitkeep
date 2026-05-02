CREATE TABLE IF NOT EXISTS site_imports (
    id             UUID PRIMARY KEY DEFAULT uuidv7(),
    site_id        UUID NOT NULL REFERENCES sites(id),
    provider       VARCHAR NOT NULL,
    status         VARCHAR NOT NULL,
    source_hash    VARCHAR,
    manifest       JSON,
    error          VARCHAR,
    bytes_total    BIGINT NOT NULL DEFAULT 0,
    bytes_received BIGINT NOT NULL DEFAULT 0,
    rows_scanned   BIGINT NOT NULL DEFAULT 0,
    rows_imported  BIGINT NOT NULL DEFAULT 0,
    created_by     UUID REFERENCES users(id),
    created_at     TIMESTAMPTZ NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL,
    validated_at   TIMESTAMPTZ,
    started_at     TIMESTAMPTZ,
    finished_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS site_imports_site_status_idx ON site_imports (site_id, status, created_at);

CREATE TABLE IF NOT EXISTS site_import_files (
    import_id      UUID NOT NULL,
    file_id        UUID NOT NULL,
    filename       VARCHAR NOT NULL,
    relative_path  VARCHAR NOT NULL,
    size_bytes     BIGINT NOT NULL,
    bytes_received BIGINT NOT NULL DEFAULT 0,
    sha256         VARCHAR,
    status         VARCHAR NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL,
    cleaned_at     TIMESTAMPTZ,
    PRIMARY KEY (import_id, file_id)
);
CREATE INDEX IF NOT EXISTS site_import_files_import_idx ON site_import_files (import_id);

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
