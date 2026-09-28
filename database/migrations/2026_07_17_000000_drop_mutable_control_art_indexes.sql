-- DuckDB 1.5.x can persistently corrupt explicit non-unique ART indexes on
-- tables updated through INSERT ... ON CONFLICT DO UPDATE. Activity summaries
-- and counters are high-churn control-plane tables; their cleanup is derived
-- from site_id/tenant_id scope columns and does not depend on these indexes.
DROP INDEX IF EXISTS site_activity_summary_tenant_idx;
DROP INDEX IF EXISTS site_activity_summary_last_hit_idx;
DROP INDEX IF EXISTS site_activity_summary_last_event_idx;
DROP INDEX IF EXISTS site_activity_hourly_counts_tenant_bucket_idx;
