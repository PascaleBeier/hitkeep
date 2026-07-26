-- Rebuild Web Vitals after the hits and events migrations have checkpointed.

DROP INDEX IF EXISTS web_vitals_site_time_idx;
DROP INDEX IF EXISTS web_vitals_site_metric_time_idx;
DROP INDEX IF EXISTS web_vitals_site_path_time_idx;
CREATE TABLE web_vitals_rebuild (
    id              UUID        NOT NULL DEFAULT uuidv7(),
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
INSERT INTO web_vitals_rebuild SELECT * FROM web_vitals;
DROP TABLE web_vitals;
ALTER TABLE web_vitals_rebuild RENAME TO web_vitals;
