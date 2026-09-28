CREATE TABLE IF NOT EXISTS users (
    id       UUID PRIMARY KEY DEFAULT uuidv7(),
    email       VARCHAR     UNIQUE NOT NULL,
    password    VARCHAR     NOT NULL, -- Should be a bcrypt hash
    created_at  TIMESTAMPTZ NOT NULL
);

-- A table to store the websites being tracked, linking them to a user.
CREATE TABLE IF NOT EXISTS sites (
    id       UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id     UUID     NOT NULL REFERENCES users(id),
    domain      VARCHAR     UNIQUE NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS sites_user_id_idx ON sites (user_id);

CREATE TABLE IF NOT EXISTS hits (
    id              UUID        PRIMARY KEY DEFAULT uuidv7(),
    site_id         UUID        NOT NULL REFERENCES sites(id),
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
    is_unique       BOOLEAN
);
CREATE INDEX IF NOT EXISTS hits_site_id_timestamp_idx ON hits (site_id, timestamp);
