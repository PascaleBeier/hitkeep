CREATE TABLE IF NOT EXISTS pending_signups (
    token VARCHAR PRIMARY KEY,
    email VARCHAR NOT NULL,
    hashed_password VARCHAR NOT NULL,
    given_name VARCHAR NOT NULL DEFAULT '',
    last_name VARCHAR NOT NULL DEFAULT '',
    team_name VARCHAR NOT NULL DEFAULT '',
    jurisdiction VARCHAR NOT NULL DEFAULT '',
    locale VARCHAR NOT NULL DEFAULT '',
    accepted_tos_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS pending_signups_email_idx ON pending_signups (email);
CREATE INDEX IF NOT EXISTS pending_signups_expires_at_idx ON pending_signups (expires_at);
