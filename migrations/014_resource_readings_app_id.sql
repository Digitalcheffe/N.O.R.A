-- 014_resource_readings_app_id.sql
-- Add optional app_id FK to resource_readings so readings can be backfilled
-- when a discovered container is linked to an NORA app (DD-7 enrichment).
ALTER TABLE resource_readings ADD COLUMN app_id TEXT REFERENCES apps(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_resource_readings_app ON resource_readings(app_id);
