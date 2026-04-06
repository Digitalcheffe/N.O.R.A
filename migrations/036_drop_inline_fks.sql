-- 036_drop_inline_fks.sql
-- Drops the inline FK columns that have been superseded by component_links.
-- Relationships are now canonical in component_links (migrated in 035).
--
-- Dropped columns:
--   infrastructure_components.parent_id
--   docker_engines.infra_component_id
--   apps.docker_engine_id
--   apps.host_component_id

PRAGMA foreign_keys = OFF;

-- ── 1. infrastructure_components — drop parent_id ────────────────────────────

CREATE TABLE infrastructure_components_new (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    ip                  TEXT,
    type                TEXT NOT NULL CHECK(type IN (
                            'proxmox_node',
                            'vm_linux',
                            'vm_windows',
                            'vm_other',
                            'linux_host',
                            'windows_host',
                            'generic_host',
                            'synology',
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
    (id, name, ip, type, collection_method, credentials, snmp_config,
     notes, enabled, last_polled_at, last_status, created_at, snmp_meta, synology_meta)
SELECT
    id, name, ip, type, collection_method, credentials, snmp_config,
    notes, enabled, last_polled_at, last_status, created_at, snmp_meta, synology_meta
FROM infrastructure_components;

DROP TABLE infrastructure_components;
ALTER TABLE infrastructure_components_new RENAME TO infrastructure_components;

CREATE INDEX IF NOT EXISTS idx_infra_components_type ON infrastructure_components(type);

-- ── 2. docker_engines — drop infra_component_id ──────────────────────────────

CREATE TABLE docker_engines_new (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    socket_type TEXT NOT NULL CHECK (socket_type IN ('local', 'remote_proxy')),
    socket_path TEXT NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT INTO docker_engines_new (id, name, socket_type, socket_path, created_at)
SELECT id, name, socket_type, socket_path, created_at
FROM docker_engines;

DROP TABLE docker_engines;
ALTER TABLE docker_engines_new RENAME TO docker_engines;

-- ── 3. apps — drop docker_engine_id and host_component_id ────────────────────

CREATE TABLE apps_new (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    token      TEXT UNIQUE NOT NULL,
    profile_id TEXT,
    config     TEXT NOT NULL DEFAULT '{}',
    rate_limit INTEGER NOT NULL DEFAULT 100,
    created_at TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT INTO apps_new (id, name, token, profile_id, config, rate_limit, created_at)
SELECT id, name, token, profile_id, config, rate_limit, created_at
FROM apps;

DROP TABLE apps;
ALTER TABLE apps_new RENAME TO apps;

PRAGMA foreign_keys = ON;
