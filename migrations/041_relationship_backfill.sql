-- 041_relationship_backfill.sql
-- Backfills component_links for relationship types that were previously stored
-- only as inline FK columns on their respective tables:
--
--   discovered_routes.app_id   → traefik_route → app
--   discovered_routes.*        → traefik → traefik_route  (route's parent component)
--   monitor_checks.app_id      → monitor → app
--
-- All inserts use INSERT OR IGNORE so existing links (e.g. container → app) are
-- never overwritten.

-- 1. traefik infra component → traefik_route (parent link for every route)
INSERT OR IGNORE INTO component_links (parent_type, parent_id, child_type, child_id, created_at)
SELECT 'traefik', infrastructure_id, 'traefik_route', id, COALESCE(created_at, datetime('now'))
FROM discovered_routes;

-- 2. traefik_route → app  (for routes that already have app_id set)
INSERT OR IGNORE INTO component_links (parent_type, parent_id, child_type, child_id, created_at)
SELECT 'traefik_route', id, 'app', app_id, COALESCE(created_at, datetime('now'))
FROM discovered_routes
WHERE app_id IS NOT NULL AND app_id != '';

-- 3. monitor → app  (for monitor checks already linked to an app)
INSERT OR IGNORE INTO component_links (parent_type, parent_id, child_type, child_id, created_at)
SELECT 'monitor', id, 'app', app_id, COALESCE(created_at, datetime('now'))
FROM monitor_checks
WHERE app_id IS NOT NULL AND app_id != '';
