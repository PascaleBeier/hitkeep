-- DuckDB explicit ART indexes can become persistently invalid after
-- ON CONFLICT updates. These Google Search Console tables are small
-- control-plane tables; their site/team cleanup remains driven by scope
-- columns and foreign keys, not by these performance indexes.
DROP INDEX IF EXISTS idx_gsc_connections_connected;
DROP INDEX IF EXISTS idx_gsc_properties_team;
DROP INDEX IF EXISTS idx_gsc_site_mappings_team;
DROP INDEX IF EXISTS idx_gsc_sync_state_next_retry;
DROP INDEX IF EXISTS idx_gsc_sync_state_team_state;
