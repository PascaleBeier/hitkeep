CREATE TABLE IF NOT EXISTS site_activity_summary (
    site_id UUID PRIMARY KEY REFERENCES sites (id),
    tenant_id UUID NOT NULL REFERENCES tenants (id),
    first_hit_at TIMESTAMPTZ,
    last_hit_at TIMESTAMPTZ,
    last_event_at TIMESTAMPTZ,
    last_hostname VARCHAR,
    last_event_name VARCHAR,
    last_automatic_event_at TIMESTAMPTZ,
    last_automatic_event_name VARCHAR,
    tracker_source VARCHAR,
    tracker_version VARCHAR,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS site_activity_summary_tenant_idx ON site_activity_summary (tenant_id);
CREATE INDEX IF NOT EXISTS site_activity_summary_last_hit_idx ON site_activity_summary (last_hit_at);
CREATE INDEX IF NOT EXISTS site_activity_summary_last_event_idx ON site_activity_summary (last_event_at);

CREATE TABLE IF NOT EXISTS site_activity_hourly_counts (
    site_id UUID NOT NULL REFERENCES sites (id),
    tenant_id UUID NOT NULL REFERENCES tenants (id),
    bucket TIMESTAMPTZ NOT NULL,
    hits BIGINT NOT NULL DEFAULT 0,
    events BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (site_id, bucket)
);

CREATE INDEX IF NOT EXISTS site_activity_hourly_counts_tenant_bucket_idx ON site_activity_hourly_counts (tenant_id, bucket);

ALTER TABLE user_preferences
ADD COLUMN IF NOT EXISTS dismissed_onboarding_at TIMESTAMPTZ;
