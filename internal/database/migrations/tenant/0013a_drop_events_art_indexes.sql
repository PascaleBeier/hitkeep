-- Rebuild events in its own committed and checkpointed migration.

DROP INDEX IF EXISTS events_site_id_timestamp_idx;
CREATE TABLE events_rebuild (
    id         UUID        NOT NULL DEFAULT uuidv7(),
    site_id    UUID        NOT NULL,
    session_id UUID        NOT NULL,
    name       VARCHAR     NOT NULL,
    properties JSON,
    timestamp  TIMESTAMPTZ NOT NULL
);
INSERT INTO events_rebuild SELECT * FROM events;
DROP TABLE events;
ALTER TABLE events_rebuild RENAME TO events;
