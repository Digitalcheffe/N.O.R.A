-- 021_snapshots.sql
-- Snapshot table for slowly-changing condition data: SSL expiry, storage
-- utilisation, OS versions, update counts, and disk health.
--
-- Snapshot scanners (REFACTOR-08) write one row per (entity_id, metric_key)
-- pair per 30-minute pass. The most recent 48 readings per pair are retained;
-- older rows are pruned by the scanner after each insert.
--
-- entity_type values match the source_type vocabulary used in the events table:
--   physical_host   — infrastructure_components with type proxmox_node/synology/etc.
--   monitor_check   — monitor_checks (SSL cert expiry)
--   snmp_host       — infrastructure_components with collection_method=snmp

CREATE TABLE snapshots (
    id             TEXT      PRIMARY KEY,
    entity_type    TEXT      NOT NULL,
    entity_id      TEXT      NOT NULL,
    metric_key     TEXT      NOT NULL,
    metric_value   TEXT      NOT NULL,
    previous_value TEXT,
    captured_at    TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_snapshots_entity_metric
    ON snapshots (entity_id, metric_key, captured_at DESC);
