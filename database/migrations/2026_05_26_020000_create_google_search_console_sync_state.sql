CREATE TABLE IF NOT EXISTS google_search_console_sync_state (
    site_id UUID PRIMARY KEY,
    team_id UUID NOT NULL,
    state VARCHAR NOT NULL DEFAULT 'idle',
    imported_start_date DATE,
    imported_end_date DATE,
    last_success_at TIMESTAMPTZ,
    last_attempt_at TIMESTAMPTZ,
    last_error_category VARCHAR DEFAULT '',
    next_retry_at TIMESTAMPTZ,
    manual BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_gsc_sync_state_team_state ON google_search_console_sync_state (team_id, state);
CREATE INDEX IF NOT EXISTS idx_gsc_sync_state_next_retry ON google_search_console_sync_state (next_retry_at);
