-- 042_route_service_health.sql
-- Adds Traefik service health columns to discovered_routes so route and
-- service data live in a single row (no need to join traefik_services).
--
-- service_status  — "enabled" / "disabled" / "" from Traefik /api/http/services
-- service_type    — "loadbalancer" / "weighted" / etc.
-- servers_total   — total backend server count
-- servers_up      — number of servers currently UP
-- servers_down    — number of servers currently DOWN

ALTER TABLE discovered_routes ADD COLUMN service_status  TEXT;
ALTER TABLE discovered_routes ADD COLUMN service_type    TEXT;
ALTER TABLE discovered_routes ADD COLUMN servers_total   INTEGER NOT NULL DEFAULT 0;
ALTER TABLE discovered_routes ADD COLUMN servers_up      INTEGER NOT NULL DEFAULT 0;
ALTER TABLE discovered_routes ADD COLUMN servers_down    INTEGER NOT NULL DEFAULT 0;
