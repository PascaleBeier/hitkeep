CREATE TABLE IF NOT EXISTS hit_rollups_hourly (
    site_id UUID NOT NULL,
    bucket TIMESTAMPTZ NOT NULL,
    pageviews BIGINT NOT NULL,
    visitors BIGINT NOT NULL,
    PRIMARY KEY (site_id, bucket)
);

CREATE INDEX IF NOT EXISTS hit_rollups_hourly_site_bucket_idx ON hit_rollups_hourly (site_id, bucket);

CREATE TABLE IF NOT EXISTS hit_rollups_daily (
    site_id UUID NOT NULL,
    bucket DATE NOT NULL,
    pageviews BIGINT NOT NULL,
    visitors BIGINT NOT NULL,
    PRIMARY KEY (site_id, bucket)
);

CREATE INDEX IF NOT EXISTS hit_rollups_daily_site_bucket_idx ON hit_rollups_daily (site_id, bucket);

CREATE TABLE IF NOT EXISTS hit_rollups_monthly (
    site_id UUID NOT NULL,
    bucket DATE NOT NULL,
    pageviews BIGINT NOT NULL,
    visitors BIGINT NOT NULL,
    PRIMARY KEY (site_id, bucket)
);

CREATE INDEX IF NOT EXISTS hit_rollups_monthly_site_bucket_idx ON hit_rollups_monthly (site_id, bucket);
