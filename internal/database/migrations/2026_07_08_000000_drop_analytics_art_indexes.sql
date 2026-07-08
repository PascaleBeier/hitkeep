-- Drop the ART indexes from the append-only analytics tables.
--
-- DuckDB keeps every PRIMARY KEY / FOREIGN KEY / CREATE INDEX as an in-memory
-- ART index that grows with row count and is never freed at runtime. On the
-- hits table these indexes measured ~60% of the database file and ~0.9 GB of
-- resident memory per 9M rows, while dashboard queries never use them (zone
-- maps prune the scans). Index maintenance also dominated insert cost.
-- DuckDB cannot drop a PRIMARY KEY in place, so the tables are rebuilt.
-- Site-transfer copies no longer rely on INSERT OR REPLACE for these tables.

DROP INDEX IF EXISTS hits_site_id_timestamp_idx;
DROP INDEX IF EXISTS hits_qr_code_id_idx;
CREATE TABLE hits_rebuild AS SELECT * FROM hits;
DROP TABLE hits;
ALTER TABLE hits_rebuild RENAME TO hits;
ALTER TABLE hits ALTER COLUMN id SET DEFAULT uuidv7();
ALTER TABLE hits ALTER COLUMN id SET NOT NULL;
ALTER TABLE hits ALTER COLUMN site_id SET NOT NULL;
ALTER TABLE hits ALTER COLUMN session_id SET NOT NULL;
ALTER TABLE hits ALTER COLUMN page_id SET NOT NULL;
ALTER TABLE hits ALTER COLUMN timestamp SET NOT NULL;
ALTER TABLE hits ALTER COLUMN path SET NOT NULL;

DROP INDEX IF EXISTS events_site_id_timestamp_idx;
CREATE TABLE events_rebuild AS SELECT * FROM events;
DROP TABLE events;
ALTER TABLE events_rebuild RENAME TO events;
ALTER TABLE events ALTER COLUMN id SET DEFAULT uuidv7();
ALTER TABLE events ALTER COLUMN id SET NOT NULL;
ALTER TABLE events ALTER COLUMN site_id SET NOT NULL;
ALTER TABLE events ALTER COLUMN session_id SET NOT NULL;
ALTER TABLE events ALTER COLUMN name SET NOT NULL;
ALTER TABLE events ALTER COLUMN timestamp SET NOT NULL;

DROP INDEX IF EXISTS web_vitals_site_time_idx;
DROP INDEX IF EXISTS web_vitals_site_metric_time_idx;
DROP INDEX IF EXISTS web_vitals_site_path_time_idx;
CREATE TABLE web_vitals_rebuild AS SELECT * FROM web_vitals;
DROP TABLE web_vitals;
ALTER TABLE web_vitals_rebuild RENAME TO web_vitals;
ALTER TABLE web_vitals ALTER COLUMN id SET DEFAULT uuidv7();
ALTER TABLE web_vitals ALTER COLUMN id SET NOT NULL;
ALTER TABLE web_vitals ALTER COLUMN site_id SET NOT NULL;
ALTER TABLE web_vitals ALTER COLUMN session_id SET NOT NULL;
ALTER TABLE web_vitals ALTER COLUMN page_id SET NOT NULL;
ALTER TABLE web_vitals ALTER COLUMN metric SET NOT NULL;
ALTER TABLE web_vitals ALTER COLUMN value SET NOT NULL;
ALTER TABLE web_vitals ALTER COLUMN rating SET NOT NULL;
ALTER TABLE web_vitals ALTER COLUMN path SET NOT NULL;
ALTER TABLE web_vitals ALTER COLUMN timestamp SET NOT NULL;
