-- Migration 038: Add first_seen_at to traefik_services.
-- first_seen_at records when NORA first observed a Traefik service.
-- Unlike last_seen it is set on INSERT and never overwritten on UPDATE.
ALTER TABLE traefik_services ADD COLUMN first_seen_at DATETIME;
