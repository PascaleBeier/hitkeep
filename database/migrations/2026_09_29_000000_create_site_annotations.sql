-- Chart annotations: team-visible notes on a site's timeline. A NULL ends_at
-- marks a single point in time; otherwise the note spans a range.
--
-- No foreign keys on purpose: DuckDB checks them eagerly and rewrites rows,
-- and site cleanup discovers this table through its site_id column.
CREATE TABLE IF NOT EXISTS site_annotations (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    site_id UUID NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ,
    body VARCHAR NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    created_by UUID,
    CHECK (length(body) BETWEEN 1 AND 280),
    CHECK (ends_at IS NULL OR ends_at >= starts_at)
);
