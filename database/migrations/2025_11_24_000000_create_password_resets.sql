CREATE TABLE IF NOT EXISTS password_resets (
    email VARCHAR NOT NULL,
    token VARCHAR NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (token)
);

CREATE INDEX IF NOT EXISTS password_resets_email_idx ON password_resets (email);