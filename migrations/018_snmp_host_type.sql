-- 018_snmp_host_type.sql
-- Add 'snmp_host' to source_type CHECK constraints so the SNMP poller can write
-- resource_readings and resource_rollups (SQLite requires table recreation to
-- change a CHECK constraint).
-- Also adds snmp_meta TEXT column to infrastructure_components for storing the
-- last-polled system identity snapshot (sysDescr, sysUpTime, sysName, CPU%, RAM,
-- per-disk bytes) as JSON.

-- ── resource_readings ─────────────────────────────────────────────────────────

CREATE TABLE resource_readings_new (
    id          TEXT      PRIMARY KEY,
    source_id   TEXT      NOT NULL,
    source_type TEXT      NOT NULL CHECK (source_type IN (
                              'docker_container', 'host', 'vm',
                              'proxmox_node', 'synology', 'snmp_host'
                          )),
    metric      TEXT      NOT NULL,
    value       REAL      NOT NULL,
    recorded_at TIMESTAMP NOT NULL,
    app_id      TEXT REFERENCES apps(id) ON DELETE SET NULL
);

INSERT INTO resource_readings_new
    SELECT id, source_id, source_type, metric, value, recorded_at, app_id
    FROM resource_readings;
DROP TABLE resource_readings;
ALTER TABLE resource_readings_new RENAME TO resource_readings;

CREATE INDEX IF NOT EXISTS idx_resource_readings_source      ON resource_readings(source_id, recorded_at);
CREATE INDEX IF NOT EXISTS idx_resource_readings_recorded_at ON resource_readings(recorded_at);
CREATE INDEX IF NOT EXISTS idx_resource_readings_app         ON resource_readings(app_id);

-- ── resource_rollups ──────────────────────────────────────────────────────────

CREATE TABLE resource_rollups_new (
    source_id    TEXT      NOT NULL,
    source_type  TEXT      NOT NULL CHECK (source_type IN (
                               'docker_container', 'host', 'vm',
                               'proxmox_node', 'synology', 'snmp_host'
                           )),
    metric       TEXT      NOT NULL,
    period_type  TEXT      NOT NULL CHECK (period_type IN ('hour', 'day')),
    period_start TIMESTAMP NOT NULL,
    avg          REAL      NOT NULL,
    min          REAL      NOT NULL,
    max          REAL      NOT NULL,
    PRIMARY KEY (source_id, metric, period_type, period_start)
);

INSERT INTO resource_rollups_new SELECT * FROM resource_rollups;
DROP TABLE resource_rollups;
ALTER TABLE resource_rollups_new RENAME TO resource_rollups;

-- ── infrastructure_components ─────────────────────────────────────────────────
-- Stores latest SNMP system identity + resource snapshot as JSON so the detail
-- API can return os_description, uptime, hostname, memory, and disk data without
-- joining multiple resource_readings rows.

ALTER TABLE infrastructure_components ADD COLUMN snmp_meta TEXT;
