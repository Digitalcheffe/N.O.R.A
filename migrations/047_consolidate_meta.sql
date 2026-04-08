-- Consolidate snmp_meta, synology_meta, traefik_meta into a single meta column.
-- Each infrastructure component has exactly one type, so these columns are
-- mutually exclusive and can safely be unified.

ALTER TABLE infrastructure_components ADD COLUMN meta TEXT;

UPDATE infrastructure_components SET meta = snmp_meta     WHERE snmp_meta     IS NOT NULL;
UPDATE infrastructure_components SET meta = synology_meta WHERE synology_meta IS NOT NULL;
UPDATE infrastructure_components SET meta = traefik_meta  WHERE traefik_meta  IS NOT NULL;

ALTER TABLE infrastructure_components DROP COLUMN snmp_meta;
ALTER TABLE infrastructure_components DROP COLUMN synology_meta;
ALTER TABLE infrastructure_components DROP COLUMN traefik_meta;
