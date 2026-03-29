-- 009_infrastructure_components.sql
-- Replaces physical_hosts + virtual_hosts with a single unified infrastructure_components table.
-- Existing rows are migrated before the old tables are dropped.
-- docker_engines is recreated to drop the virtual_host_id FK and gain infra_component_id.

-- ── 1. Create unified table ───────────────────────────────────────────────────

CREATE TABLE infrastructure_components (
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
                            'docker_engine'
                        )),
    collection_method   TEXT NOT NULL CHECK(collection_method IN (
                            'proxmox_api',
                            'synology_api',
                            'snmp',
                            'docker_socket',
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

CREATE INDEX idx_infra_components_parent ON infrastructure_components(parent_id);
CREATE INDEX idx_infra_components_type   ON infrastructure_components(type);

-- ── 2. Migrate physical_hosts rows ───────────────────────────────────────────
-- IDs are preserved so resource_readings.source_id values remain valid.

INSERT INTO infrastructure_components (id, name, ip, type, collection_method, parent_id, notes, enabled, created_at)
SELECT
    id,
    name,
    ip,
    CASE type WHEN 'proxmox_node' THEN 'proxmox_node' ELSE 'bare_metal' END,
    'none',
    NULL,
    COALESCE(notes, ''),
    1,
    created_at
FROM physical_hosts;

-- ── 3. Migrate virtual_hosts rows ────────────────────────────────────────────
-- parent_id maps to the migrated infrastructure_components.id from physical_hosts.

INSERT INTO infrastructure_components (id, name, ip, type, collection_method, parent_id, notes, enabled, created_at)
SELECT
    id,
    name,
    ip,
    CASE type WHEN 'lxc' THEN 'lxc' ELSE 'vm' END,
    'none',
    physical_host_id,
    '',
    1,
    created_at
FROM virtual_hosts;

-- ── 4. Recreate docker_engines without the virtual_host_id FK ────────────────
-- SQLite enforces FK constraints (PRAGMA foreign_keys=ON), so dropping virtual_hosts
-- while docker_engines still references it would break DML. Recreate the table
-- with infra_component_id instead, mapping from the preserved IDs.

CREATE TABLE docker_engines_new (
    id                 TEXT PRIMARY KEY,
    infra_component_id TEXT REFERENCES infrastructure_components(id) ON DELETE SET NULL,
    name               TEXT NOT NULL,
    socket_type        TEXT NOT NULL CHECK (socket_type IN ('local', 'remote_proxy')),
    socket_path        TEXT NOT NULL,
    created_at         TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT INTO docker_engines_new (id, infra_component_id, name, socket_type, socket_path, created_at)
SELECT id, virtual_host_id, name, socket_type, socket_path, created_at
FROM docker_engines;

DROP TABLE docker_engines;
ALTER TABLE docker_engines_new RENAME TO docker_engines;

-- ── 5. Drop old tables ───────────────────────────────────────────────────────
-- docker_engines no longer references virtual_hosts, so these drops are safe.

DROP TABLE IF EXISTS virtual_hosts;
DROP TABLE IF EXISTS physical_hosts;
