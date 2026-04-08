-- Add per-server URL→status map to discovered_routes so the Traefik detail
-- page can display backend hosts without a separate traefik_services table.
ALTER TABLE discovered_routes ADD COLUMN servers_json TEXT;
