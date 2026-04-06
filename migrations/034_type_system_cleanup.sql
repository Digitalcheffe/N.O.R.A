-- 034_type_system_cleanup.sql
-- Cleans up legacy infrastructure component types:
--
--   Removed:  vm, lxc, bare_metal, virtual_host (component type)
--   Added:    vm_linux, vm_windows, vm_other
--
-- Data migrations:
--   bare_metal  → generic_host
--   vm          → vm_other    (Proxmox poller now sets vm_linux/vm_windows/vm_other from ostype)
--   lxc rows    → deleted     (LXC is no longer a tracked entity type)
--
-- Also updates source_type CHECK constraints in:
--   events            (virtual_host → vm_other, physical_host kept for scanner compat)
--   resource_readings (vm → vm_other)
--   resource_rollups  (vm → vm_other)

PRAGMA foreign_keys = OFF;

-- Drop any leftover _new tables from a previously interrupted migration run.
DROP TABLE IF EXISTS infrastructure_components_new;
DROP TABLE IF EXISTS events_new;
DROP TABLE IF EXISTS resource_readings_new;
DROP TABLE IF EXISTS resource_rollups_new;

-- ── 1. infrastructure_components ─────────────────────────────────────────────

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
    parent_id           TEXT REFERENCES infrastructure_components_new(id) ON DELETE SET NULL,
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

-- Migrate data: bare_metal → generic_host, vm → vm_other, lxc rows deleted.
INSERT INTO infrastructure_components_new
    (id, name, ip, type, collection_method, parent_id, credentials, snmp_config,
     notes, enabled, last_polled_at, last_status, created_at, snmp_meta, synology_meta)
SELECT
    id, name, ip,
    CASE type
        WHEN 'bare_metal' THEN 'generic_host'
        WHEN 'vm'         THEN 'vm_other'
        ELSE type
    END,
    collection_method, parent_id, credentials, snmp_config,
    notes, enabled, last_polled_at, last_status, created_at, snmp_meta, synology_meta
FROM infrastructure_components
WHERE type != 'lxc';

DROP TABLE infrastructure_components;
ALTER TABLE infrastructure_components_new RENAME TO infrastructure_components;

CREATE INDEX IF NOT EXISTS idx_infra_components_parent ON infrastructure_components(parent_id);
CREATE INDEX IF NOT EXISTS idx_infra_components_type   ON infrastructure_components(type);

-- ── 2. events ─────────────────────────────────────────────────────────────────
-- Replaces virtual_host with vm_other. Keeps physical_host as valid so existing
-- scanner code (which hardcodes "physical_host") continues to compile without
-- mass-updating every scanner file. That cleanup is a separate task.

CREATE TABLE events_new (
    id          TEXT      PRIMARY KEY,
    level       TEXT      NOT NULL CHECK (level IN ('debug', 'info', 'warn', 'error', 'critical')),
    source_name TEXT      NOT NULL,
    source_type TEXT      NOT NULL CHECK (source_type IN (
                                'app',
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
                                'portainer',
                                'container',
                                'monitor_check',
                                'physical_host',
                                'system'
                            )),
    source_id   TEXT,
    title       TEXT      NOT NULL,
    payload     TEXT,
    created_at  TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT INTO events_new (id, level, source_name, source_type, source_id, title, payload, created_at)
SELECT id, level, source_name,
    CASE source_type
        WHEN 'virtual_host' THEN 'vm_other'
        ELSE source_type
    END,
    source_id, title, payload, created_at
FROM events;

DROP TABLE events;
ALTER TABLE events_new RENAME TO events;

CREATE INDEX IF NOT EXISTS idx_events_created_at  ON events (created_at);
CREATE INDEX IF NOT EXISTS idx_events_level       ON events (level);
CREATE INDEX IF NOT EXISTS idx_events_source_type ON events (source_type);
CREATE INDEX IF NOT EXISTS idx_events_source_id   ON events (source_id);

-- ── 3. resource_readings ──────────────────────────────────────────────────────

CREATE TABLE resource_readings_new (
    id          TEXT      PRIMARY KEY,
    source_id   TEXT      NOT NULL,
    source_type TEXT      NOT NULL CHECK (source_type IN (
                              'docker_container',
                              'host',
                              'vm_linux',
                              'vm_windows',
                              'vm_other',
                              'proxmox_node',
                              'synology',
                              'snmp_host'
                          )),
    metric      TEXT      NOT NULL,
    value       REAL      NOT NULL,
    recorded_at TIMESTAMP NOT NULL,
    app_id      TEXT REFERENCES apps(id) ON DELETE SET NULL
);

INSERT INTO resource_readings_new (id, source_id, source_type, metric, value, recorded_at, app_id)
SELECT id, source_id,
    CASE source_type
        WHEN 'vm' THEN 'vm_other'
        ELSE source_type
    END,
    metric, value, recorded_at, app_id
FROM resource_readings;

DROP TABLE resource_readings;
ALTER TABLE resource_readings_new RENAME TO resource_readings;

CREATE INDEX IF NOT EXISTS idx_resource_readings_source      ON resource_readings(source_id, recorded_at);
CREATE INDEX IF NOT EXISTS idx_resource_readings_recorded_at ON resource_readings(recorded_at);
CREATE INDEX IF NOT EXISTS idx_resource_readings_app         ON resource_readings(app_id);

-- ── 4. resource_rollups ───────────────────────────────────────────────────────

CREATE TABLE resource_rollups_new (
    source_id    TEXT      NOT NULL,
    source_type  TEXT      NOT NULL CHECK (source_type IN (
                               'docker_container',
                               'host',
                               'vm_linux',
                               'vm_windows',
                               'vm_other',
                               'proxmox_node',
                               'synology',
                               'snmp_host'
                           )),
    metric       TEXT      NOT NULL,
    period_type  TEXT      NOT NULL CHECK (period_type IN ('hour', 'day')),
    period_start TIMESTAMP NOT NULL,
    avg          REAL      NOT NULL,
    min          REAL      NOT NULL,
    max          REAL      NOT NULL,
    PRIMARY KEY (source_id, metric, period_type, period_start)
);

INSERT INTO resource_rollups_new
SELECT source_id,
    CASE source_type
        WHEN 'vm' THEN 'vm_other'
        ELSE source_type
    END,
    metric, period_type, period_start, avg, min, max
FROM resource_rollups;

DROP TABLE resource_rollups;
ALTER TABLE resource_rollups_new RENAME TO resource_rollups;

PRAGMA foreign_keys = ON;
