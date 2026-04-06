-- 039_containers_name_key.sql
-- Rebuilds discovered_containers with two goals:
--
--  1. Change the uniqueness key from (infra_component_id, container_id) to
--     (infra_component_id, container_name).  Docker container IDs change on
--     every rebuild; the name stays stable.  This lets an upsert on name
--     update the existing row (refreshing container_id) rather than creating
--     a duplicate.
--
--  2. Add source_type (TEXT) to record where the container was discovered
--     ("docker_engine" or "portainer").  This allows queries and the UI to
--     show provenance without joining infrastructure_components.
--
-- Existing rows are deduplicated: for each (infra_component_id, container_name)
-- pair, the row with an app_id is preferred; ties are broken by latest
-- last_seen_at.  source_type is backfilled from infrastructure_components.type.
--
-- component_links rows are also backfilled so every discovered container has a
-- canonical parent link.

-- ── 1. New table ──────────────────────────────────────────────────────────────

CREATE TABLE discovered_containers_new (
    id                     TEXT PRIMARY KEY,
    infra_component_id     TEXT NOT NULL REFERENCES infrastructure_components(id) ON DELETE CASCADE,
    source_type            TEXT NOT NULL DEFAULT '',        -- 'docker_engine' | 'portainer'
    container_id           TEXT NOT NULL DEFAULT '',        -- Docker container hash; refreshed on discovery
    container_name         TEXT NOT NULL,
    image                  TEXT NOT NULL,
    status                 TEXT NOT NULL,                   -- running | stopped | exited
    app_id                 TEXT REFERENCES apps(id) ON DELETE SET NULL,
    profile_suggestion     TEXT,
    suggestion_confidence  INTEGER,
    last_seen_at           DATETIME NOT NULL,
    created_at             DATETIME NOT NULL DEFAULT (datetime('now')),
    image_digest           TEXT,
    registry_digest        TEXT,
    image_update_available INTEGER NOT NULL DEFAULT 0,
    image_last_checked_at  DATETIME,
    ports                  TEXT,
    labels                 TEXT,
    volumes                TEXT,
    networks               TEXT,
    restart_policy         TEXT,
    docker_created_at      DATETIME,
    UNIQUE(infra_component_id, container_name)
);

-- ── 2. Migrate + deduplicate ──────────────────────────────────────────────────
-- For each (infra_component_id, container_name) keep the single best row:
--   priority 1 – has an app_id (user-linked containers must not lose their link)
--   priority 2 – latest last_seen_at

INSERT INTO discovered_containers_new
    (id, infra_component_id, source_type, container_id, container_name,
     image, status, app_id, profile_suggestion, suggestion_confidence,
     last_seen_at, created_at,
     image_digest, registry_digest, image_update_available, image_last_checked_at,
     ports, labels, volumes, networks, restart_policy, docker_created_at)
SELECT
    dc.id,
    dc.infra_component_id,
    COALESCE(ic.type, '')   AS source_type,
    dc.container_id,
    dc.container_name,
    dc.image,
    dc.status,
    dc.app_id,
    dc.profile_suggestion,
    dc.suggestion_confidence,
    dc.last_seen_at,
    dc.created_at,
    dc.image_digest,
    dc.registry_digest,
    COALESCE(dc.image_update_available, 0),
    dc.image_last_checked_at,
    dc.ports,
    dc.labels,
    dc.volumes,
    dc.networks,
    dc.restart_policy,
    dc.docker_created_at
FROM discovered_containers dc
LEFT JOIN infrastructure_components ic ON dc.infra_component_id = ic.id
WHERE dc.rowid = (
    SELECT dc2.rowid
    FROM   discovered_containers dc2
    WHERE  dc2.infra_component_id = dc.infra_component_id
      AND  dc2.container_name     = dc.container_name
    ORDER BY
           (dc2.app_id IS NOT NULL) DESC,
           dc2.last_seen_at DESC
    LIMIT 1
);

-- ── 3. Swap tables ────────────────────────────────────────────────────────────

DROP TABLE discovered_containers;
ALTER TABLE discovered_containers_new RENAME TO discovered_containers;

-- ── 4. Indexes ────────────────────────────────────────────────────────────────

CREATE INDEX IF NOT EXISTS idx_discovered_containers_infra
    ON discovered_containers(infra_component_id);

CREATE INDEX IF NOT EXISTS idx_discovered_containers_app
    ON discovered_containers(app_id);

CREATE INDEX IF NOT EXISTS idx_discovered_containers_source
    ON discovered_containers(source_type);

-- ── 5. Backfill component_links ───────────────────────────────────────────────
-- Ensure every existing container has a parent link keyed by its source component.

INSERT OR IGNORE INTO component_links
    (parent_type, parent_id, child_type, child_id, created_at)
SELECT
    ic.type,
    ic.id,
    'container',
    dc.id,
    dc.created_at
FROM discovered_containers dc
JOIN infrastructure_components ic ON dc.infra_component_id = ic.id;
