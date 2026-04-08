-- Migrate traefik_overview into a traefik_meta JSON column on infrastructure_components.

ALTER TABLE infrastructure_components ADD COLUMN traefik_meta TEXT;

UPDATE infrastructure_components
SET traefik_meta = (
    SELECT json_object(
        'version',           version,
        'routers_total',     routers_total,
        'routers_errors',    routers_errors,
        'routers_warnings',  routers_warnings,
        'services_total',    services_total,
        'services_errors',   services_errors,
        'middlewares_total', middlewares_total,
        'updated_at',        updated_at
    )
    FROM traefik_overview
    WHERE component_id = infrastructure_components.id
)
WHERE type = 'traefik';

DROP TABLE IF EXISTS traefik_overview;
