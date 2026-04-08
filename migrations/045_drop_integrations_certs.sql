-- 045_drop_integrations_certs.sql
-- Retires the legacy infrastructure_integrations cert-cache system.
-- Cert data was never used after Traefik components (infrastructure_components)
-- took over route/cert discovery via discovered_routes.

-- Clear traefik-mode SSL checks so they become standalone checks.
UPDATE monitor_checks SET ssl_source = NULL, integration_id = NULL WHERE ssl_source = 'traefik';

-- Drop columns added for traefik-mode SSL checks.
ALTER TABLE monitor_checks DROP COLUMN ssl_source;
ALTER TABLE monitor_checks DROP COLUMN integration_id;

-- Drop cert and integration tables (CASCADE handled by FK constraints).
DROP TABLE IF EXISTS traefik_certs;
DROP TABLE IF EXISTS traefik_component_certs;
DROP TABLE IF EXISTS infrastructure_integrations;
