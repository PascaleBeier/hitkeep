CREATE TABLE IF NOT EXISTS remember_me_tokens (
    token VARCHAR(64) PRIMARY KEY,
    user_id UUID NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users (id)
);

CREATE INDEX idx_remember_me_tokens_user_id ON remember_me_tokens (user_id);