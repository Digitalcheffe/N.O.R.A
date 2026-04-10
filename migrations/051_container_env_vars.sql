-- AP-05: store environment variables captured during container inspect.
-- env_vars is a JSON array of "KEY=VALUE" strings, populated by the Portainer
-- enrichment worker via InspectContainer during each poll cycle.
ALTER TABLE discovered_containers ADD COLUMN env_vars TEXT;
