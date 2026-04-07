-- 040_container_component_links.sql
-- Normalises container and app→container relationships into component_links.
--
-- Prior to this migration:
--   discovered_containers.infra_component_id  — container's parent (portainer/docker_engine/etc)
--   discovered_containers.app_id              — which app this container belongs to
-- Neither of these were reflected in component_links, so the topology chain
-- (app → container → portainer → vm → proxmox) was only partially in the table.
--
-- After this migration component_links is the single source of truth:
--   container  → infra_component  (portainer, docker_engine, etc.)
--   app        → container        (replaces old app → docker_engine direct link)

-- 1. Backfill container → parent infra_component links.
INSERT OR IGNORE INTO component_links (parent_type, parent_id, child_type, child_id, created_at)
SELECT ic.type, ic.id, 'container', dc.id, COALESCE(dc.created_at, datetime('now'))
FROM discovered_containers dc
JOIN infrastructure_components ic ON dc.infra_component_id = ic.id;

-- 2. For apps already in component_links via old docker_engine_id / host_component_id
--    backfill: switch their parent to the container when one exists.
UPDATE component_links
SET parent_type = 'container',
    parent_id   = (
        SELECT dc.id
        FROM discovered_containers dc
        WHERE dc.app_id = component_links.child_id
        LIMIT 1
    )
WHERE child_type = 'app'
  AND EXISTS (
    SELECT 1 FROM discovered_containers dc WHERE dc.app_id = component_links.child_id
  );

-- 3. Insert app → container links for apps not yet in component_links.
INSERT OR IGNORE INTO component_links (parent_type, parent_id, child_type, child_id, created_at)
SELECT 'container', dc.id, 'app', dc.app_id, COALESCE(dc.created_at, datetime('now'))
FROM discovered_containers dc
WHERE dc.app_id IS NOT NULL;
