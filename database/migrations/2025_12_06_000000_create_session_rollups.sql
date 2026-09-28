CREATE TABLE IF NOT EXISTS session_rollups_hourly (
    site_id UUID NOT NULL,
    bucket TIMESTAMPTZ NOT NULL,
    sessions BIGINT NOT NULL,
    bounced_sessions BIGINT NOT NULL,
    duration_sum_seconds DOUBLE NOT NULL,
    pageviews BIGINT NOT NULL,
    PRIMARY KEY (site_id, bucket)
);

CREATE INDEX IF NOT EXISTS session_rollups_hourly_site_bucket_idx ON session_rollups_hourly (site_id, bucket);

CREATE TABLE IF NOT EXISTS session_rollups_daily (
    site_id UUID NOT NULL,
    bucket DATE NOT NULL,
    sessions BIGINT NOT NULL,
    bounced_sessions BIGINT NOT NULL,
    duration_sum_seconds DOUBLE NOT NULL,
    pageviews BIGINT NOT NULL,
    PRIMARY KEY (site_id, bucket)
);

CREATE INDEX IF NOT EXISTS session_rollups_daily_site_bucket_idx ON session_rollups_daily (site_id, bucket);

CREATE TABLE IF NOT EXISTS session_rollups_monthly (
    site_id UUID NOT NULL,
    bucket DATE NOT NULL,
    sessions BIGINT NOT NULL,
    bounced_sessions BIGINT NOT NULL,
    duration_sum_seconds DOUBLE NOT NULL,
    pageviews BIGINT NOT NULL,
    PRIMARY KEY (site_id, bucket)
);

CREATE INDEX IF NOT EXISTS session_rollups_monthly_site_bucket_idx ON session_rollups_monthly (site_id, bucket);
