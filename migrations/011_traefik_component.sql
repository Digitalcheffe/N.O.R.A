-- 011_traefik_component.sql
-- Adds 'traefik' as a component type and 'traefik_api' as a collection method.
-- Creates traefik_component_certs and traefik_routes tables linked to components.
-- Adds source_component_id to monitor_checks for Traefik-owned SSL checks.

-- ── 1. Disable FK enforcement for table recreation ────────────────────────────

PRAGMA foreign_keys = OFF;

-- ── 2. Recreate infrastructure_components with traefik type ──────────────────

CREATE TABLE infrastructure_components_new (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    ip                  TEXT,
    type                TEXT NOT NULL CHECK(type IN (
                            'proxmox_node',
                            'synology',
                            'vm',
                            'lxc',
                            'bare_metal',
                            'windows_host',
                            'docker_engine',
                            'traefik'
                        )),
    collection_method   TEXT NOT NULL CHECK(collection_method IN (
                            'proxmox_api',
                            'synology_api',
                            'snmp',
                            'docker_socket',
                            'traefik_api',
                            'none'
                        )) DEFAULT 'none',
    parent_id           TEXT REFERENCES infrastructure_components(id) ON DELETE SET NULL,
    credentials         TEXT,
    snmp_config         TEXT,
    notes               TEXT,
    enabled             INTEGER NOT NULL DEFAULT 1,
    last_polled_at      TIMESTAMP,
    last_status         TEXT CHECK(last_status IN ('online', 'degraded', 'offline', 'unknown')) DEFAULT 'unknown',
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO infrastructure_components_new
SELECT * FROM infrastructure_components;

DROP TABLE infrastructure_components;
ALTER TABLE infrastructure_components_new RENAME TO infrastructure_components;

CREATE INDEX idx_infra_components_parent ON infrastructure_components(parent_id);
CREATE INDEX idx_infra_components_type   ON infrastructure_components(type);

-- ── 3. Re-enable FK enforcement ───────────────────────────────────────────────

PRAGMA foreign_keys = ON;

-- ── 4. Add source_component_id to monitor_checks ─────────────────────────────

ALTER TABLE monitor_checks ADD COLUMN source_component_id TEXT;
CREATE INDEX idx_monitor_checks_component ON monitor_checks(source_component_id);

-- ── 5. Create traefik_component_certs ─────────────────────────────────────────

CREATE TABLE traefik_component_certs (
    id            TEXT PRIMARY KEY,
    component_id  TEXT NOT NULL REFERENCES infrastructure_components(id) ON DELETE CASCADE,
    domain        TEXT NOT NULL,
    issuer        TEXT,
    expires_at    TIMESTAMP,
    sans          TEXT NOT NULL DEFAULT '[]',
    last_seen_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(component_id, domain)
);

CREATE INDEX idx_traefik_comp_certs_component ON traefik_component_certs(component_id);

-- ── 6. Create traefik_routes ──────────────────────────────────────────────────

CREATE TABLE traefik_routes (
    id            TEXT PRIMARY KEY,
    component_id  TEXT NOT NULL REFERENCES infrastructure_components(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    rule          TEXT NOT NULL DEFAULT '',
    service       TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'enabled',
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(component_id, name)
);

CREATE INDEX idx_traefik_routes_component ON traefik_routes(component_id);
