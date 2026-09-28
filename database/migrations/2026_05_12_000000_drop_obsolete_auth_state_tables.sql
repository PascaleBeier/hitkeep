DROP INDEX IF EXISTS password_resets_email_idx;
DROP INDEX IF EXISTS passkey_login_challenges_user_id_idx;

DROP TABLE IF EXISTS password_resets;
DROP TABLE IF EXISTS user_totp_pending_setup;
DROP TABLE IF EXISTS user_passkey_challenges;
DROP TABLE IF EXISTS passkey_login_challenges;
