CREATE TABLE IF NOT EXISTS user_recovery_codes (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id UUID NOT NULL REFERENCES users(id),
    code_hash VARCHAR NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    UNIQUE(user_id, code_hash)
);

CREATE INDEX IF NOT EXISTS user_recovery_codes_user_id_idx ON user_recovery_codes(user_id);
