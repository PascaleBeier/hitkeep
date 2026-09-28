ALTER TABLE user_passkeys
    ADD COLUMN IF NOT EXISTS sign_count BIGINT;

UPDATE user_passkeys
SET sign_count = 0
WHERE sign_count IS NULL;

CREATE TABLE IF NOT EXISTS passkey_login_challenges (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id UUID,
    challenge VARCHAR NOT NULL,
    remember_me BOOLEAN NOT NULL DEFAULT FALSE,
    flow VARCHAR NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS passkey_login_challenges_user_id_idx ON passkey_login_challenges(user_id);
