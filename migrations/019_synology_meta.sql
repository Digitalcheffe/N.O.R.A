-- 019_synology_meta.sql
-- Adds synology_meta TEXT column to infrastructure_components for storing the
-- last-polled Synology DSM snapshot (model, DSM version, hostname, uptime,
-- temperature, CPU, memory, volumes, disks, update status) as JSON.
-- Follows the same pattern as snmp_meta (migration 018).

ALTER TABLE infrastructure_components ADD COLUMN synology_meta TEXT;
