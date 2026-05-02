DROP INDEX IF EXISTS imported_event_properties_daily_site_event_date_idx;
DROP INDEX IF EXISTS imported_event_properties_daily_key_idx;
DROP INDEX IF EXISTS imported_event_properties_daily_import_idx;

UPDATE imported_event_properties_daily
SET event_name = ''
WHERE event_name IS NULL;

ALTER TABLE imported_event_properties_daily
ALTER COLUMN event_name SET DEFAULT '';

ALTER TABLE imported_event_properties_daily
ALTER COLUMN event_name SET NOT NULL;

CREATE INDEX IF NOT EXISTS imported_event_properties_daily_site_event_date_idx ON imported_event_properties_daily (site_id, event_name, date);
CREATE INDEX IF NOT EXISTS imported_event_properties_daily_key_idx ON imported_event_properties_daily (site_id, property_key, date);
CREATE INDEX IF NOT EXISTS imported_event_properties_daily_import_idx ON imported_event_properties_daily (import_id);

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
