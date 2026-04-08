-- Retire the docker_engines table. Each row is migrated to infrastructure_components
-- using the same ID so all existing component_links remain valid without any update.

INSERT INTO infrastructure_components (id, name, type, collection_method, enabled, last_status, created_at)
SELECT de.id, de.name, 'docker_engine', 'docker_socket', 1, 'unknown', de.created_at
FROM docker_engines de
WHERE NOT EXISTS (
    SELECT 1 FROM infrastructure_components ic WHERE ic.id = de.id
);

DROP TABLE IF EXISTS docker_engines;
