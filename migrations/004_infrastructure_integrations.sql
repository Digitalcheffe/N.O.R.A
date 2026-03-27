-- T-34: Infrastructure integrations — Traefik cert discovery
-- Creates tables for infra integrations and the Traefik cert cache.

CREATE TABLE infrastructure_integrations (
    id              TEXT PRIMARY KEY,
    type            TEXT NOT NULL,          -- traefik (future: npm, caddy)
    name            TEXT NOT NULL,
    api_url         TEXT NOT NULL,
    api_key         TEXT,                   -- nullable: set if dashboard auth is enabled
    enabled         INTEGER NOT NULL DEFAULT 1,
    last_synced_at  DATETIME,
    last_status     TEXT,                   -- ok | error
    last_error      TEXT,
    created_at      DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE traefik_certs (
    id              TEXT PRIMARY KEY,
    integration_id  TEXT NOT NULL REFERENCES infrastructure_integrations(id) ON DELETE CASCADE,
    domain          TEXT NOT NULL,
    issuer          TEXT,
    expires_at      DATETIME,
    sans            TEXT,                   -- JSON array of SANs
    last_seen_at    DATETIME NOT NULL,
    UNIQUE(integration_id, domain)
);
