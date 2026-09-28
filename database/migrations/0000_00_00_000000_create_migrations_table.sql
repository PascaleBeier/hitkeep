CREATE TABLE IF NOT EXISTS migrations (
    migration VARCHAR PRIMARY KEY,

    applied_at TIMESTAMPTZ NOT NULL
);