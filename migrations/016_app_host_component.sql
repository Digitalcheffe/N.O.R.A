-- Link apps to an infrastructure component (the host they run on).
-- host_component_id is nullable; deleting the component sets it to NULL.
ALTER TABLE apps ADD COLUMN host_component_id TEXT
    REFERENCES infrastructure_components(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_apps_host_component_id ON apps(host_component_id);
