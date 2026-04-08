-- Fix app → container links that were incorrectly stored as docker_engine/portainer → app.
-- When LinkContainerApp was called, SetDiscoveredContainerApp wrote the correct
-- container → app link, but the subsequent SetParent call overwrote it with the engine.
-- This migration restores the correct link for all affected apps using correlated subqueries.
UPDATE component_links
SET
  parent_type = 'container',
  parent_id   = (
    SELECT dc.id
    FROM discovered_containers AS dc
    WHERE dc.app_id = component_links.child_id
    LIMIT 1
  )
WHERE component_links.child_type = 'app'
  AND component_links.parent_type IN ('docker_engine', 'portainer')
  AND EXISTS (
    SELECT 1
    FROM discovered_containers AS dc
    WHERE dc.app_id = component_links.child_id
  );
