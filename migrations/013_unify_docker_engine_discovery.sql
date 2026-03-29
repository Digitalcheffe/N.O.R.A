-- 013_unify_docker_engine_discovery.sql
-- Replaces docker_engine_id FK with infra_component_id in discovered_containers.
-- docker_engines was a separate table; discovery now hangs off infrastructure_components
-- (type = 'docker_engine') directly, which is the canonical infrastructure record.

-- Drop the old table (data is ephemeral — re-discovered on startup).
DROP TABLE IF EXISTS discovered_containers;

CREATE TABLE discovered_containers (
    id                    TEXT PRIMARY KEY,
    infra_component_id    TEXT NOT NULL REFERENCES infrastructure_components(id) ON DELETE CASCADE,
    container_id          TEXT NOT NULL,              -- Docker container ID (short)
    container_name        TEXT NOT NULL,              -- name without leading /
    image                 TEXT NOT NULL,              -- full image:tag
    status                TEXT NOT NULL,              -- running | stopped | exited
    app_id                TEXT REFERENCES apps(id) ON DELETE SET NULL,
    profile_suggestion    TEXT,                       -- profile_id if matched, null if unknown
    suggestion_confidence INTEGER,                    -- 0-100
    last_seen_at          DATETIME NOT NULL,
    created_at            DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(infra_component_id, container_id)
);

CREATE INDEX IF NOT EXISTS idx_discovered_containers_infra ON discovered_containers(infra_component_id);
CREATE INDEX IF NOT EXISTS idx_discovered_containers_app   ON discovered_containers(app_id);
