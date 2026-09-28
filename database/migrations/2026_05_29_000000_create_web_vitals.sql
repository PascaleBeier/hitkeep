CREATE TABLE IF NOT EXISTS web_vitals (
    id              UUID        PRIMARY KEY DEFAULT uuidv7(),
    site_id         UUID        NOT NULL,
    session_id      UUID        NOT NULL,
    page_id         UUID        NOT NULL,
    metric          VARCHAR     NOT NULL CHECK (metric IN ('LCP', 'INP', 'CLS', 'FCP', 'TTFB')),
    metric_id       VARCHAR,
    value           DOUBLE      NOT NULL,
    rating          VARCHAR     NOT NULL CHECK (rating IN ('good', 'needs_improvement', 'poor')),
    path            VARCHAR     NOT NULL,
    navigation_type VARCHAR,
    timestamp       TIMESTAMPTZ NOT NULL,
    tracker_source  VARCHAR,
    tracker_version VARCHAR
);

CREATE INDEX IF NOT EXISTS web_vitals_site_time_idx ON web_vitals (site_id, timestamp);
CREATE INDEX IF NOT EXISTS web_vitals_site_metric_time_idx ON web_vitals (site_id, metric, timestamp);
CREATE INDEX IF NOT EXISTS web_vitals_site_path_time_idx ON web_vitals (site_id, path, timestamp);
