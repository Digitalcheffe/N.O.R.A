-- Migration 037: Add container enrichment columns to discovered_containers.
-- These columns capture port bindings, labels, mounts, networks, restart policy,
-- and Docker's own creation timestamp — written by the discovery scanner and
-- image-update poller so the UI can display richer container detail.
ALTER TABLE discovered_containers ADD COLUMN ports TEXT;
ALTER TABLE discovered_containers ADD COLUMN labels TEXT;
ALTER TABLE discovered_containers ADD COLUMN volumes TEXT;
ALTER TABLE discovered_containers ADD COLUMN networks TEXT;
ALTER TABLE discovered_containers ADD COLUMN restart_policy TEXT;
ALTER TABLE discovered_containers ADD COLUMN docker_created_at DATETIME;
