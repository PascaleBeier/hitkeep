CREATE TABLE IF NOT EXISTS user_preferences (
    user_id UUID PRIMARY KEY REFERENCES users(id),
    default_locale VARCHAR NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
