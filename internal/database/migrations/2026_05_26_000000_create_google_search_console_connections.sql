CREATE TABLE IF NOT EXISTS google_search_console_connections (
    team_id UUID PRIMARY KEY,
    connected_by_user_id UUID,
    google_account_email VARCHAR DEFAULT '',
    google_account_id VARCHAR DEFAULT '',
    access_token VARCHAR DEFAULT '',
    refresh_token VARCHAR DEFAULT '',
    token_type VARCHAR DEFAULT '',
    scope VARCHAR DEFAULT '',
    token_expiry TIMESTAMP,
    connected BOOLEAN DEFAULT false,
    connected_at TIMESTAMP,
    disconnected_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT now(),
    updated_at TIMESTAMP DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_gsc_connections_connected ON google_search_console_connections (connected, updated_at DESC);
