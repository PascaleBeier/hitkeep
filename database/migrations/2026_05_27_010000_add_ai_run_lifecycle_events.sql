ALTER TABLE ai_runs ADD COLUMN IF NOT EXISTS lifecycle_events_json JSON DEFAULT '[]';
