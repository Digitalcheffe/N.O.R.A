-- 001_init.sql
-- Full NORA schema. All tables created here — including v2 stubs — so there are never gaps.

CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    email         TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'member',
    created_at    TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE physical_hosts (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    ip         TEXT NOT NULL,
    type       TEXT NOT NULL CHECK (type IN ('bare_metal', 'proxmox_node')),
    notes      TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE virtual_hosts (
    id               TEXT PRIMARY KEY,
    physical_host_id TEXT REFERENCES physical_hosts (id) ON DELETE SET NULL,
    name             TEXT NOT NULL,
    ip               TEXT NOT NULL,
    type             TEXT NOT NULL CHECK (type IN ('vm', 'lxc', 'wsl')),
    created_at       TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE docker_engines (
    id              TEXT PRIMARY KEY,
    virtual_host_id TEXT REFERENCES virtual_hosts (id) ON DELETE SET NULL,
    name            TEXT NOT NULL,
    socket_type     TEXT NOT NULL CHECK (socket_type IN ('local', 'remote_proxy')),
    socket_path     TEXT NOT NULL,
    created_at      TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE apps (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    token            TEXT UNIQUE NOT NULL,
    profile_id       TEXT,
    docker_engine_id TEXT REFERENCES docker_engines (id) ON DELETE SET NULL,
    config           TEXT NOT NULL DEFAULT '{}',
    rate_limit       INTEGER NOT NULL DEFAULT 100,
    created_at       TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE events (
    id           TEXT PRIMARY KEY,
    app_id       TEXT NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    received_at  TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    severity     TEXT NOT NULL CHECK (severity IN ('debug', 'info', 'warn', 'error', 'critical')),
    display_text TEXT NOT NULL,
    raw_payload  TEXT NOT NULL DEFAULT '{}',
    fields       TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_events_app_id     ON events (app_id);
CREATE INDEX idx_events_received_at ON events (received_at);
CREATE INDEX idx_events_severity   ON events (severity);

CREATE TABLE monitor_checks (
    id              TEXT PRIMARY KEY,
    app_id          TEXT REFERENCES apps (id) ON DELETE SET NULL,
    name            TEXT NOT NULL,
    type            TEXT NOT NULL CHECK (type IN ('ping', 'url', 'ssl')),
    target          TEXT NOT NULL,
    interval_secs   INTEGER NOT NULL DEFAULT 300,
    expected_status INTEGER,
    ssl_warn_days   INTEGER NOT NULL DEFAULT 30,
    ssl_crit_days   INTEGER NOT NULL DEFAULT 7,
    enabled         INTEGER NOT NULL DEFAULT 1,
    last_checked_at TIMESTAMP,
    last_status     TEXT CHECK (last_status IN ('up', 'warn', 'down')),
    last_result     TEXT,
    created_at      TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE rollups (
    app_id     TEXT    NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    year       INTEGER NOT NULL,
    month      INTEGER NOT NULL,
    event_type TEXT    NOT NULL,
    severity   TEXT    NOT NULL,
    count      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (app_id, year, month, event_type, severity)
);

CREATE TABLE metrics (
    app_id            TEXT      NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    period            TIMESTAMP NOT NULL,
    events_per_hour   INTEGER   NOT NULL DEFAULT 0,
    avg_payload_bytes INTEGER   NOT NULL DEFAULT 0,
    peak_per_minute   INTEGER   NOT NULL DEFAULT 0,
    PRIMARY KEY (app_id, period)
);

CREATE TABLE resource_readings (
    id          TEXT PRIMARY KEY,
    source_id   TEXT      NOT NULL,
    source_type TEXT      NOT NULL CHECK (source_type IN ('docker_container', 'host', 'vm')),
    metric      TEXT      NOT NULL CHECK (metric IN ('cpu_percent', 'mem_percent', 'mem_bytes', 'disk_percent')),
    value       REAL      NOT NULL,
    recorded_at TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_resource_readings_source ON resource_readings (source_id, recorded_at);

CREATE TABLE resource_rollups (
    source_id    TEXT      NOT NULL,
    source_type  TEXT      NOT NULL CHECK (source_type IN ('docker_container', 'host', 'vm')),
    metric       TEXT      NOT NULL CHECK (metric IN ('cpu_percent', 'mem_percent', 'mem_bytes', 'disk_percent')),
    period_type  TEXT      NOT NULL CHECK (period_type IN ('hour', 'day')),
    period_start TIMESTAMP NOT NULL,
    avg          REAL      NOT NULL,
    min          REAL      NOT NULL,
    max          REAL      NOT NULL,
    PRIMARY KEY (source_id, metric, period_type, period_start)
);

-- v2 stub — schema present from day one, implementation deferred
CREATE TABLE alert_rules (
    id              TEXT PRIMARY KEY,
    app_id          TEXT REFERENCES apps (id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    conditions      TEXT NOT NULL DEFAULT '[]',
    condition_logic TEXT NOT NULL DEFAULT 'AND',
    notif_title     TEXT NOT NULL DEFAULT '',
    notif_body      TEXT NOT NULL DEFAULT '',
    enabled         INTEGER NOT NULL DEFAULT 1,
    created_at      TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- v2 stub — Web Push subscriptions
CREATE TABLE web_push_subscriptions (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    endpoint   TEXT NOT NULL,
    p256dh     TEXT NOT NULL,
    auth       TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
