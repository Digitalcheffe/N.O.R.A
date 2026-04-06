-- 035_component_links.sql
-- Introduces component_links as the canonical parent-child relationship table.
-- Replaces the scattered inline FK columns:
--   infrastructure_components.parent_id
--   docker_engines.infra_component_id
--   apps.host_component_id
--   apps.docker_engine_id
--
-- Each child has exactly one parent (PRIMARY KEY on child_type, child_id).
-- parent_type / child_type are entity type strings matching the type values
-- used across infrastructure_components, docker_engines, apps, and
-- discovered_containers.
--
-- Inline FK columns are dropped in migration 036 once this table is populated.

CREATE TABLE IF NOT EXISTS component_links (
    parent_type  TEXT NOT NULL,
    parent_id    TEXT NOT NULL,
    child_type   TEXT NOT NULL,
    child_id     TEXT NOT NULL,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY  (child_type, child_id)
);

CREATE INDEX IF NOT EXISTS idx_component_links_parent
    ON component_links(parent_type, parent_id);

-- ── Backfill from infrastructure_components.parent_id ────────────────────────
INSERT OR IGNORE INTO component_links (parent_type, parent_id, child_type, child_id, created_at)
SELECT
    p.type,
    p.id,
    c.type,
    c.id,
    COALESCE(c.created_at, datetime('now'))
FROM infrastructure_components c
JOIN infrastructure_components p ON c.parent_id = p.id
WHERE c.parent_id IS NOT NULL;

-- ── Backfill from docker_engines.infra_component_id ──────────────────────────
INSERT OR IGNORE INTO component_links (parent_type, parent_id, child_type, child_id, created_at)
SELECT
    ic.type,
    ic.id,
    'docker_engine',
    de.id,
    COALESCE(de.created_at, datetime('now'))
FROM docker_engines de
JOIN infrastructure_components ic ON de.infra_component_id = ic.id
WHERE de.infra_component_id IS NOT NULL AND de.infra_component_id != '';

-- ── Backfill from apps.docker_engine_id ──────────────────────────────────────
INSERT OR IGNORE INTO component_links (parent_type, parent_id, child_type, child_id, created_at)
SELECT
    'docker_engine',
    a.docker_engine_id,
    'app',
    a.id,
    COALESCE(a.created_at, datetime('now'))
FROM apps a
WHERE a.docker_engine_id IS NOT NULL AND a.docker_engine_id != '';

-- ── Backfill from apps.host_component_id ─────────────────────────────────────
-- docker_engine_id takes precedence (already inserted above via ON CONFLICT
-- with PRIMARY KEY); host_component_id is used only when no docker_engine_id
-- is set, so this INSERT OR IGNORE is safe.
INSERT OR IGNORE INTO component_links (parent_type, parent_id, child_type, child_id, created_at)
SELECT
    ic.type,
    ic.id,
    'app',
    a.id,
    COALESCE(a.created_at, datetime('now'))
FROM apps a
JOIN infrastructure_components ic ON a.host_component_id = ic.id
WHERE a.host_component_id IS NOT NULL
  AND (a.docker_engine_id IS NULL OR a.docker_engine_id = '');
