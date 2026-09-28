ALTER TABLE user_passkeys
    ADD COLUMN IF NOT EXISTS credential_json VARCHAR;
