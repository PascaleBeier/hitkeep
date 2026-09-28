CREATE TABLE IF NOT EXISTS goal_rollups_hourly (
    site_id UUID NOT NULL,
    goal_id UUID NOT NULL,
    bucket TIMESTAMPTZ NOT NULL,
    conversions BIGINT NOT NULL,
    PRIMARY KEY (site_id, goal_id, bucket)
);

CREATE INDEX IF NOT EXISTS goal_rollups_hourly_site_bucket_idx ON goal_rollups_hourly (site_id, bucket);
CREATE INDEX IF NOT EXISTS goal_rollups_hourly_goal_bucket_idx ON goal_rollups_hourly (goal_id, bucket);

CREATE TABLE IF NOT EXISTS goal_rollups_daily (
    site_id UUID NOT NULL,
    goal_id UUID NOT NULL,
    bucket DATE NOT NULL,
    conversions BIGINT NOT NULL,
    PRIMARY KEY (site_id, goal_id, bucket)
);

CREATE INDEX IF NOT EXISTS goal_rollups_daily_site_bucket_idx ON goal_rollups_daily (site_id, bucket);
CREATE INDEX IF NOT EXISTS goal_rollups_daily_goal_bucket_idx ON goal_rollups_daily (goal_id, bucket);

CREATE TABLE IF NOT EXISTS goal_rollups_monthly (
    site_id UUID NOT NULL,
    goal_id UUID NOT NULL,
    bucket DATE NOT NULL,
    conversions BIGINT NOT NULL,
    PRIMARY KEY (site_id, goal_id, bucket)
);

CREATE INDEX IF NOT EXISTS goal_rollups_monthly_site_bucket_idx ON goal_rollups_monthly (site_id, bucket);
CREATE INDEX IF NOT EXISTS goal_rollups_monthly_goal_bucket_idx ON goal_rollups_monthly (goal_id, bucket);

CREATE TABLE IF NOT EXISTS funnel_rollups_hourly (
    site_id UUID NOT NULL,
    funnel_id UUID NOT NULL,
    bucket TIMESTAMPTZ NOT NULL,
    entries BIGINT NOT NULL,
    completions BIGINT NOT NULL,
    PRIMARY KEY (site_id, funnel_id, bucket)
);

CREATE INDEX IF NOT EXISTS funnel_rollups_hourly_site_bucket_idx ON funnel_rollups_hourly (site_id, bucket);
CREATE INDEX IF NOT EXISTS funnel_rollups_hourly_funnel_bucket_idx ON funnel_rollups_hourly (funnel_id, bucket);

CREATE TABLE IF NOT EXISTS funnel_rollups_daily (
    site_id UUID NOT NULL,
    funnel_id UUID NOT NULL,
    bucket DATE NOT NULL,
    entries BIGINT NOT NULL,
    completions BIGINT NOT NULL,
    PRIMARY KEY (site_id, funnel_id, bucket)
);

CREATE INDEX IF NOT EXISTS funnel_rollups_daily_site_bucket_idx ON funnel_rollups_daily (site_id, bucket);
CREATE INDEX IF NOT EXISTS funnel_rollups_daily_funnel_bucket_idx ON funnel_rollups_daily (funnel_id, bucket);

CREATE TABLE IF NOT EXISTS funnel_rollups_monthly (
    site_id UUID NOT NULL,
    funnel_id UUID NOT NULL,
    bucket DATE NOT NULL,
    entries BIGINT NOT NULL,
    completions BIGINT NOT NULL,
    PRIMARY KEY (site_id, funnel_id, bucket)
);

CREATE INDEX IF NOT EXISTS funnel_rollups_monthly_site_bucket_idx ON funnel_rollups_monthly (site_id, bucket);
CREATE INDEX IF NOT EXISTS funnel_rollups_monthly_funnel_bucket_idx ON funnel_rollups_monthly (funnel_id, bucket);
