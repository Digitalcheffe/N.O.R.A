-- 026_portainer_component.sql
-- Adds 'portainer' as a component type and 'portainer_api' as a collection method.
-- Portainer provides container visibility across one or more Docker environments
-- and is the authoritative source for image update status (DD-8).

-- ── 1. Disable FK enforcement for table recreation ────────────────────────────

PRAGMA foreign_keys = OFF;

-- ── 2. Recreate infrastructure_components with portainer type ─────────────────
-- Column order matches the existing table (id, name, ip, type, collection_method,
-- parent_id, credentials, snmp_config, notes, enabled, last_polled_at, last_status,
-- created_at, snmp_meta, synology_meta) so the SELECT * copy is safe.

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
                            'linux_host',
                            'windows_host',
                            'generic_host',
                            'docker_engine',
                            'traefik',
                            'portainer'
                        )),
    collection_method   TEXT NOT NULL CHECK(collection_method IN (
                            'proxmox_api',
                            'synology_api',
                            'snmp',
                            'docker_socket',
                            'traefik_api',
                            'portainer_api',
                            'none'
                        )) DEFAULT 'none',
    parent_id           TEXT REFERENCES infrastructure_components(id) ON DELETE SET NULL,
    credentials         TEXT,
    snmp_config         TEXT,
    notes               TEXT,
    enabled             INTEGER NOT NULL DEFAULT 1,
    last_polled_at      TIMESTAMP,
    last_status         TEXT CHECK(last_status IN ('online', 'degraded', 'offline', 'unknown')) DEFAULT 'unknown',
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    snmp_meta           TEXT,
    synology_meta       TEXT
);

INSERT INTO infrastructure_components_new
    (id, name, ip, type, collection_method, parent_id, credentials, snmp_config,
     notes, enabled, last_polled_at, last_status, created_at, snmp_meta, synology_meta)
SELECT
    id, name, ip, type, collection_method, parent_id, credentials, snmp_config,
    notes, enabled, last_polled_at, last_status, created_at, snmp_meta, synology_meta
FROM infrastructure_components;

DROP TABLE infrastructure_components;
ALTER TABLE infrastructure_components_new RENAME TO infrastructure_components;

CREATE INDEX idx_infra_components_parent ON infrastructure_components(parent_id);
CREATE INDEX idx_infra_components_type   ON infrastructure_components(type);

-- ── 3. Re-enable FK enforcement ───────────────────────────────────────────────

PRAGMA foreign_keys = ON;
