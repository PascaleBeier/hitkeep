-- Tenant-scoped analytics tables.
-- This migration runs on per-tenant DuckDB files (not the shared hitkeep.db).

CREATE TABLE IF NOT EXISTS migrations (
    migration  VARCHAR NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL
);

-- Sites reference table (subset of shared DB; needed for foreign key targets).
CREATE TABLE IF NOT EXISTS sites (
    id                  UUID PRIMARY KEY,
    domain              VARCHAR NOT NULL,
    data_retention_days INTEGER DEFAULT 365
);

-- Hits
CREATE TABLE IF NOT EXISTS hits (
    id              UUID        PRIMARY KEY DEFAULT uuidv7(),
    site_id         UUID        NOT NULL,
    session_id      UUID        NOT NULL,
    page_id         UUID        NOT NULL,
    timestamp       TIMESTAMPTZ NOT NULL,
    path            VARCHAR     NOT NULL,
    hostname        VARCHAR,
    referrer        VARCHAR,
    user_agent      VARCHAR,
    viewport_width  INT,
    viewport_height INT,
    screen_width    INT,
    screen_height   INT,
    language        VARCHAR,
    is_unique       BOOLEAN,
    country_code    VARCHAR,
    utm_source      VARCHAR,
    utm_medium      VARCHAR,
    utm_campaign    VARCHAR,
    utm_term        VARCHAR,
    utm_content     VARCHAR
);
CREATE INDEX IF NOT EXISTS hits_site_id_timestamp_idx ON hits (site_id, timestamp);

-- Events
CREATE TABLE IF NOT EXISTS events (
    id         UUID        PRIMARY KEY DEFAULT uuidv7(),
    site_id    UUID        NOT NULL,
    session_id UUID        NOT NULL,
    name       VARCHAR     NOT NULL,
    properties JSON,
    timestamp  TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS events_site_id_timestamp_idx ON events (site_id, timestamp);

-- Goals
CREATE TABLE IF NOT EXISTS goals (
    id         UUID        PRIMARY KEY DEFAULT uuidv7(),
    site_id    UUID        NOT NULL,
    name       VARCHAR     NOT NULL,
    type       VARCHAR     NOT NULL CHECK (type IN ('event', 'path')),
    value      VARCHAR     NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS goals_site_id_idx ON goals (site_id);

-- Funnels
CREATE TABLE IF NOT EXISTS funnels (
    id         UUID        PRIMARY KEY DEFAULT uuidv7(),
    site_id    UUID        NOT NULL,
    name       VARCHAR     NOT NULL,
    steps      JSON        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS funnels_site_id_idx ON funnels (site_id);

-- Hit rollups
CREATE TABLE IF NOT EXISTS hit_rollups_hourly (
    site_id   UUID        NOT NULL,
    bucket    TIMESTAMPTZ NOT NULL,
    pageviews BIGINT      NOT NULL,
    visitors  BIGINT      NOT NULL,
    PRIMARY KEY (site_id, bucket)
);
CREATE INDEX IF NOT EXISTS hit_rollups_hourly_site_bucket_idx ON hit_rollups_hourly (site_id, bucket);

CREATE TABLE IF NOT EXISTS hit_rollups_daily (
    site_id   UUID   NOT NULL,
    bucket    DATE   NOT NULL,
    pageviews BIGINT NOT NULL,
    visitors  BIGINT NOT NULL,
    PRIMARY KEY (site_id, bucket)
);
CREATE INDEX IF NOT EXISTS hit_rollups_daily_site_bucket_idx ON hit_rollups_daily (site_id, bucket);

CREATE TABLE IF NOT EXISTS hit_rollups_monthly (
    site_id   UUID   NOT NULL,
    bucket    DATE   NOT NULL,
    pageviews BIGINT NOT NULL,
    visitors  BIGINT NOT NULL,
    PRIMARY KEY (site_id, bucket)
);
CREATE INDEX IF NOT EXISTS hit_rollups_monthly_site_bucket_idx ON hit_rollups_monthly (site_id, bucket);

-- Session rollups
CREATE TABLE IF NOT EXISTS session_rollups_hourly (
    site_id              UUID        NOT NULL,
    bucket               TIMESTAMPTZ NOT NULL,
    sessions             BIGINT      NOT NULL,
    bounced_sessions     BIGINT      NOT NULL,
    duration_sum_seconds DOUBLE      NOT NULL,
    pageviews            BIGINT      NOT NULL,
    PRIMARY KEY (site_id, bucket)
);
CREATE INDEX IF NOT EXISTS session_rollups_hourly_site_bucket_idx ON session_rollups_hourly (site_id, bucket);

CREATE TABLE IF NOT EXISTS session_rollups_daily (
    site_id              UUID   NOT NULL,
    bucket               DATE   NOT NULL,
    sessions             BIGINT NOT NULL,
    bounced_sessions     BIGINT NOT NULL,
    duration_sum_seconds DOUBLE NOT NULL,
    pageviews            BIGINT NOT NULL,
    PRIMARY KEY (site_id, bucket)
);
CREATE INDEX IF NOT EXISTS session_rollups_daily_site_bucket_idx ON session_rollups_daily (site_id, bucket);

CREATE TABLE IF NOT EXISTS session_rollups_monthly (
    site_id              UUID   NOT NULL,
    bucket               DATE   NOT NULL,
    sessions             BIGINT NOT NULL,
    bounced_sessions     BIGINT NOT NULL,
    duration_sum_seconds DOUBLE NOT NULL,
    pageviews            BIGINT NOT NULL,
    PRIMARY KEY (site_id, bucket)
);
CREATE INDEX IF NOT EXISTS session_rollups_monthly_site_bucket_idx ON session_rollups_monthly (site_id, bucket);

-- Goal rollups
CREATE TABLE IF NOT EXISTS goal_rollups_hourly (
    site_id     UUID        NOT NULL,
    goal_id     UUID        NOT NULL,
    bucket      TIMESTAMPTZ NOT NULL,
    conversions BIGINT      NOT NULL,
    PRIMARY KEY (site_id, goal_id, bucket)
);
CREATE INDEX IF NOT EXISTS goal_rollups_hourly_site_bucket_idx ON goal_rollups_hourly (site_id, bucket);
CREATE INDEX IF NOT EXISTS goal_rollups_hourly_goal_bucket_idx ON goal_rollups_hourly (goal_id, bucket);

CREATE TABLE IF NOT EXISTS goal_rollups_daily (
    site_id     UUID   NOT NULL,
    goal_id     UUID   NOT NULL,
    bucket      DATE   NOT NULL,
    conversions BIGINT NOT NULL,
    PRIMARY KEY (site_id, goal_id, bucket)
);
CREATE INDEX IF NOT EXISTS goal_rollups_daily_site_bucket_idx ON goal_rollups_daily (site_id, bucket);
CREATE INDEX IF NOT EXISTS goal_rollups_daily_goal_bucket_idx ON goal_rollups_daily (goal_id, bucket);

CREATE TABLE IF NOT EXISTS goal_rollups_monthly (
    site_id     UUID   NOT NULL,
    goal_id     UUID   NOT NULL,
    bucket      DATE   NOT NULL,
    conversions BIGINT NOT NULL,
    PRIMARY KEY (site_id, goal_id, bucket)
);
CREATE INDEX IF NOT EXISTS goal_rollups_monthly_site_bucket_idx ON goal_rollups_monthly (site_id, bucket);
CREATE INDEX IF NOT EXISTS goal_rollups_monthly_goal_bucket_idx ON goal_rollups_monthly (goal_id, bucket);

-- Funnel rollups
CREATE TABLE IF NOT EXISTS funnel_rollups_hourly (
    site_id     UUID        NOT NULL,
    funnel_id   UUID        NOT NULL,
    bucket      TIMESTAMPTZ NOT NULL,
    entries     BIGINT      NOT NULL,
    completions BIGINT      NOT NULL,
    PRIMARY KEY (site_id, funnel_id, bucket)
);
CREATE INDEX IF NOT EXISTS funnel_rollups_hourly_site_bucket_idx ON funnel_rollups_hourly (site_id, bucket);
CREATE INDEX IF NOT EXISTS funnel_rollups_hourly_funnel_bucket_idx ON funnel_rollups_hourly (funnel_id, bucket);

CREATE TABLE IF NOT EXISTS funnel_rollups_daily (
    site_id     UUID   NOT NULL,
    funnel_id   UUID   NOT NULL,
    bucket      DATE   NOT NULL,
    entries     BIGINT NOT NULL,
    completions BIGINT NOT NULL,
    PRIMARY KEY (site_id, funnel_id, bucket)
);
CREATE INDEX IF NOT EXISTS funnel_rollups_daily_site_bucket_idx ON funnel_rollups_daily (site_id, bucket);
CREATE INDEX IF NOT EXISTS funnel_rollups_daily_funnel_bucket_idx ON funnel_rollups_daily (funnel_id, bucket);

CREATE TABLE IF NOT EXISTS funnel_rollups_monthly (
    site_id     UUID   NOT NULL,
    funnel_id   UUID   NOT NULL,
    bucket      DATE   NOT NULL,
    entries     BIGINT NOT NULL,
    completions BIGINT NOT NULL,
    PRIMARY KEY (site_id, funnel_id, bucket)
);
CREATE INDEX IF NOT EXISTS funnel_rollups_monthly_site_bucket_idx ON funnel_rollups_monthly (site_id, bucket);
CREATE INDEX IF NOT EXISTS funnel_rollups_monthly_funnel_bucket_idx ON funnel_rollups_monthly (funnel_id, bucket);

-- Analytics macros
CREATE OR REPLACE MACRO hk_referrer(ref) AS (
    CASE
        WHEN ref IS NULL OR ref = '' THEN '(Direct)'
        WHEN ref LIKE 'http%' THEN regexp_extract(ref, 'https?://([^/]+)', 1)
        ELSE ref
    END
);

CREATE OR REPLACE MACRO hk_device(viewport_width) AS (
    CASE
        WHEN viewport_width < 576 THEN 'Mobile'
        WHEN viewport_width < 992 THEN 'Tablet'
        ELSE 'Desktop'
    END
);

CREATE OR REPLACE MACRO hk_country(country_code) AS (
    COALESCE(NULLIF(country_code, ''), '(Unknown)')
);
