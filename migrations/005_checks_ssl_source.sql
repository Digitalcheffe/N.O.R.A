-- T-34: Add ssl_source and integration_id columns to monitor_checks.
-- ssl_source: "traefik" reads expiry from cert cache; "standalone" dials TLS directly.
-- integration_id: links a traefik-mode SSL check to its integration row.

ALTER TABLE monitor_checks ADD COLUMN ssl_source    TEXT;
ALTER TABLE monitor_checks ADD COLUMN integration_id TEXT REFERENCES infrastructure_integrations(id);
