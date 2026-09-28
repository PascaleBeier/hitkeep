CREATE TABLE IF NOT EXISTS rollup_dirty_buckets (
    site_id     UUID        NOT NULL,
    rollup_type VARCHAR     NOT NULL CHECK (rollup_type IN ('hit', 'session')),
    bucket_unit VARCHAR     NOT NULL CHECK (bucket_unit IN ('hour', 'day', 'month')),
    bucket      TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (site_id, rollup_type, bucket_unit, bucket)
);

CREATE INDEX IF NOT EXISTS rollup_dirty_buckets_site_updated_idx
ON rollup_dirty_buckets (site_id, updated_at);
