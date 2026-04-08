-- 044_drop_traefik_services.sql
-- Drops the traefik_services table. Service health is now derived at query time
-- from discovered_routes grouped by service_name (see ListServicesForComponent).

DROP TABLE IF EXISTS traefik_services;
