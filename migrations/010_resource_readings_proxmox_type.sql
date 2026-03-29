-- 010_resource_readings_proxmox_type.sql
-- Extend source_type CHECK constraints in resource_readings and resource_rollups
-- to include 'proxmox_node' and 'synology' (for future Synology poller).
-- SQLite does not support ALTER COLUMN, so we recreate both tables.

-- ── resource_readings ─────────────────────────────────────────────────────────

CREATE TABLE resource_readings_new (
    id          TEXT      PRIMARY KEY,
    source_id   TEXT      NOT NULL,
    source_type TEXT      NOT NULL CHECK (source_type IN (
                              'docker_container', 'host', 'vm',
                              'proxmox_node', 'synology'
                          )),
    metric      TEXT      NOT NULL,
    value       REAL      NOT NULL,
    recorded_at TIMESTAMP NOT NULL
);

INSERT INTO resource_readings_new SELECT * FROM resource_readings;
DROP TABLE resource_readings;
ALTER TABLE resource_readings_new RENAME TO resource_readings;

CREATE INDEX IF NOT EXISTS idx_resource_readings_source      ON resource_readings(source_id, recorded_at);
CREATE INDEX IF NOT EXISTS idx_resource_readings_recorded_at ON resource_readings(recorded_at);

-- ── resource_rollups ──────────────────────────────────────────────────────────

CREATE TABLE resource_rollups_new (
    source_id    TEXT      NOT NULL,
    source_type  TEXT      NOT NULL CHECK (source_type IN (
                               'docker_container', 'host', 'vm',
                               'proxmox_node', 'synology'
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
