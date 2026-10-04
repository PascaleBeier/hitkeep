-- The property whose older imported days all carry query-free totals. Manual
-- syncs re-import those days once per property, then skip them. Empty until
-- a sync completes that backfill.
ALTER TABLE google_search_console_sync_state ADD COLUMN IF NOT EXISTS totals_backfilled_property_uri VARCHAR DEFAULT '';
