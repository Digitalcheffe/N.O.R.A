-- Add DNS check type: expand type CHECK constraint and add dns-specific columns.
-- SQLite does not support ALTER COLUMN, so we recreate the table.

PRAGMA foreign_keys = OFF;

CREATE TABLE monitor_checks_new (
    id                  TEXT PRIMARY KEY,
    app_id              TEXT REFERENCES apps (id) ON DELETE SET NULL,
    name                TEXT NOT NULL,
    type                TEXT NOT NULL CHECK (type IN ('ping', 'url', 'ssl', 'dns')),
    target              TEXT NOT NULL,
    interval_secs       INTEGER NOT NULL DEFAULT 300,
    expected_status     INTEGER,
    ssl_warn_days       INTEGER NOT NULL DEFAULT 30,
    ssl_crit_days       INTEGER NOT NULL DEFAULT 7,
    ssl_source          TEXT,
    integration_id      TEXT,
    source_component_id TEXT,
    skip_tls_verify     INTEGER NOT NULL DEFAULT 0,
    dns_record_type     TEXT,
    dns_expected_value  TEXT,
    enabled             INTEGER NOT NULL DEFAULT 1,
    last_checked_at     TIMESTAMP,
    last_status         TEXT,
    last_result         TEXT,
    created_at          TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT INTO monitor_checks_new
    SELECT id, app_id, name, type, target,
           interval_secs, expected_status,
           ssl_warn_days, ssl_crit_days, ssl_source, integration_id,
           source_component_id, skip_tls_verify,
           NULL, NULL,
           enabled, last_checked_at, last_status, last_result, created_at
    FROM monitor_checks;

DROP TABLE monitor_checks;
ALTER TABLE monitor_checks_new RENAME TO monitor_checks;

PRAGMA foreign_keys = ON;
