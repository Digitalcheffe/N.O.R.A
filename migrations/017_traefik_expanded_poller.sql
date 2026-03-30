-- 017_traefik_expanded_poller.sql
-- Expands the Traefik integration with overview health, enriched router status,
-- and per-service backend health tracking (Infra-10).

-- ── traefik_overview ──────────────────────────────────────────────────────────
-- One row per Traefik component. Replaced each poll cycle.

CREATE TABLE IF NOT EXISTS traefik_overview (
    component_id        TEXT PRIMARY KEY,
    version             TEXT NOT NULL DEFAULT '',
    routers_total       INTEGER NOT NULL DEFAULT 0,
    routers_errors      INTEGER NOT NULL DEFAULT 0,
    routers_warnings    INTEGER NOT NULL DEFAULT 0,
    services_total      INTEGER NOT NULL DEFAULT 0,
    services_errors     INTEGER NOT NULL DEFAULT 0,
    middlewares_total   INTEGER NOT NULL DEFAULT 0,
    updated_at          DATETIME NOT NULL
);

-- ── traefik_services ──────────────────────────────────────────────────────────
-- One row per (component, service). Replaced each poll cycle.

CREATE TABLE IF NOT EXISTS traefik_services (
    id                  TEXT PRIMARY KEY,          -- "{component_id}:{service_name}"
    component_id        TEXT NOT NULL,
    service_name        TEXT NOT NULL,
    service_type        TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL DEFAULT 'enabled',
    server_count        INTEGER NOT NULL DEFAULT 0,
    servers_up          INTEGER NOT NULL DEFAULT 0,
    servers_down        INTEGER NOT NULL DEFAULT 0,
    server_status_json  TEXT NOT NULL DEFAULT '{}',
    last_seen           DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_traefik_services_component
    ON traefik_services(component_id);

-- ── discovered_routes: add enriched router columns ────────────────────────────
-- router_name and domain already exist from migration 012.
-- Each ALTER TABLE runs exactly once (migration is tracked), so IF NOT EXISTS
-- is intentionally omitted for SQLite compatibility.

ALTER TABLE discovered_routes ADD COLUMN router_status TEXT NOT NULL DEFAULT 'enabled';
ALTER TABLE discovered_routes ADD COLUMN provider TEXT;
ALTER TABLE discovered_routes ADD COLUMN entry_points TEXT;       -- JSON array
ALTER TABLE discovered_routes ADD COLUMN has_tls_resolver INTEGER NOT NULL DEFAULT 0;
ALTER TABLE discovered_routes ADD COLUMN cert_resolver_name TEXT;
ALTER TABLE discovered_routes ADD COLUMN service_name TEXT;       -- full service name including provider suffix
