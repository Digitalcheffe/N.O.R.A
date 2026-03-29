-- 012_discovery_tables.sql
-- Adds discovered_containers and discovered_routes tables for DD-1.
-- discovered_containers: one row per container found via a Docker Engine component.
-- discovered_routes: one row per HTTP router found via a Traefik component.

-- ── discovered_containers ─────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS discovered_containers (
    id                    TEXT PRIMARY KEY,
    docker_engine_id      TEXT NOT NULL REFERENCES docker_engines(id) ON DELETE CASCADE,
    container_id          TEXT NOT NULL,              -- Docker container ID (short)
    container_name        TEXT NOT NULL,              -- name without leading /
    image                 TEXT NOT NULL,              -- full image:tag
    status                TEXT NOT NULL,              -- running | stopped | exited
    app_id                TEXT REFERENCES apps(id) ON DELETE SET NULL,
    profile_suggestion    TEXT,                       -- profile_id if matched, null if unknown
    suggestion_confidence INTEGER,                    -- 0-100
    last_seen_at          DATETIME NOT NULL,
    created_at            DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(docker_engine_id, container_id)
);

CREATE INDEX IF NOT EXISTS idx_discovered_containers_engine ON discovered_containers(docker_engine_id);
CREATE INDEX IF NOT EXISTS idx_discovered_containers_app    ON discovered_containers(app_id);

-- ── discovered_routes ─────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS discovered_routes (
    id                TEXT PRIMARY KEY,
    infrastructure_id TEXT NOT NULL,              -- FK to infrastructure_components.id
    router_name       TEXT NOT NULL,
    rule              TEXT NOT NULL,              -- raw Traefik rule e.g. Host(`sonarr.example.com`)
    domain            TEXT,                       -- parsed domain from rule
    backend_service   TEXT,                       -- Traefik service name
    container_id      TEXT REFERENCES discovered_containers(id) ON DELETE SET NULL,
    app_id            TEXT REFERENCES apps(id) ON DELETE SET NULL,
    ssl_expiry        DATETIME,                   -- pulled from Traefik cert store if available
    ssl_issuer        TEXT,
    last_seen_at      DATETIME NOT NULL,
    created_at        DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(infrastructure_id, router_name)
);

CREATE INDEX IF NOT EXISTS idx_discovered_routes_infra ON discovered_routes(infrastructure_id);
CREATE INDEX IF NOT EXISTS idx_discovered_routes_app   ON discovered_routes(app_id);
