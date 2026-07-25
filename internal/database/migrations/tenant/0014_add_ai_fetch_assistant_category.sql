-- AI agent category (ai_training_crawler, ai_assistant, ...) from the embedded
-- AI agent master list. Nullable: rows ingested before the category dimension
-- existed stay NULL and are reported as unknown.
ALTER TABLE ai_fetches ADD COLUMN IF NOT EXISTS assistant_category VARCHAR;
