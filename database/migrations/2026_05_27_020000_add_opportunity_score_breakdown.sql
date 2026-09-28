ALTER TABLE opportunities
ADD COLUMN IF NOT EXISTS score_breakdown_json JSON DEFAULT '{}';
