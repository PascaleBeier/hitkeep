ALTER TABLE users
    ADD COLUMN password_login_enabled BOOLEAN DEFAULT TRUE;

CREATE TABLE IF NOT EXISTS social_identities (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id UUID NOT NULL REFERENCES users (id),
    provider VARCHAR NOT NULL CHECK (provider IN ('google', 'github', 'microsoft')),
    subject VARCHAR NOT NULL,
    observed_email VARCHAR NOT NULL DEFAULT '',
    linked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,
    UNIQUE (provider, subject),
    UNIQUE (user_id, provider)
);

CREATE INDEX IF NOT EXISTS social_identities_user_idx ON social_identities (user_id);

CREATE TABLE IF NOT EXISTS pending_social_confirmations (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    token_hash VARCHAR UNIQUE NOT NULL,
    provider VARCHAR NOT NULL CHECK (provider IN ('google', 'github', 'microsoft')),
    subject VARCHAR NOT NULL,
    observed_email VARCHAR NOT NULL DEFAULT '',
    target_email VARCHAR NOT NULL,
    target_user_id UUID REFERENCES users (id),
    team_name VARCHAR NOT NULL DEFAULT '',
    jurisdiction VARCHAR NOT NULL DEFAULT '',
    locale VARCHAR NOT NULL DEFAULT '',
    plan_code VARCHAR NOT NULL DEFAULT 'free',
    billing_interval VARCHAR NOT NULL DEFAULT 'monthly',
    accepted_tos_at TIMESTAMPTZ,
    return_path VARCHAR NOT NULL DEFAULT '/dashboard',
    remember_me BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, subject)
);

CREATE INDEX IF NOT EXISTS pending_social_confirmations_expires_idx ON pending_social_confirmations (expires_at);
CREATE INDEX IF NOT EXISTS pending_social_confirmations_user_idx ON pending_social_confirmations (target_user_id);
