ALTER TABLE google_search_console_sync_state
ADD COLUMN IF NOT EXISTS last_error_message VARCHAR DEFAULT '';
