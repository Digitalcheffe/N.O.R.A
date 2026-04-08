-- 043_drop_traefik_routes.sql
-- Drops the traefik_routes table. Routes are now exclusively managed via
-- discovered_routes which carries full service health, container/app linking,
-- entry points, TLS info, and server counts.

DROP TABLE IF EXISTS traefik_routes;
