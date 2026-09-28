CREATE TABLE IF NOT EXISTS user_totp_pending_setup (
    user_id UUID PRIMARY KEY REFERENCES users(id),
    secret VARCHAR NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS user_totp_factors (
    user_id UUID PRIMARY KEY REFERENCES users(id),
    secret VARCHAR NOT NULL,
    enabled_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS user_passkey_challenges (
    user_id UUID PRIMARY KEY REFERENCES users(id),
    challenge VARCHAR NOT NULL,
    requested_name VARCHAR,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS user_passkeys (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id UUID NOT NULL REFERENCES users(id),
    name VARCHAR NOT NULL,
    credential_id VARCHAR UNIQUE NOT NULL,
    public_key VARCHAR,
    transports_json VARCHAR,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS user_passkeys_user_id_idx ON user_passkeys(user_id);
