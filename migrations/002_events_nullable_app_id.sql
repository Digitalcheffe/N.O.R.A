-- 002_events_nullable_app_id.sql
-- Allow events with no associated app (e.g. Docker container events for unregistered containers).
-- SQLite does not support ALTER COLUMN, so we recreate the table.

CREATE TABLE events_new (
    id           TEXT PRIMARY KEY,
    app_id       TEXT REFERENCES apps (id) ON DELETE CASCADE,
    received_at  TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    severity     TEXT NOT NULL CHECK (severity IN ('debug', 'info', 'warn', 'error', 'critical')),
    display_text TEXT NOT NULL,
    raw_payload  TEXT NOT NULL DEFAULT '{}',
    fields       TEXT NOT NULL DEFAULT '{}'
);

INSERT INTO events_new SELECT * FROM events;
DROP TABLE events;
ALTER TABLE events_new RENAME TO events;

CREATE INDEX idx_events_app_id      ON events (app_id);
CREATE INDEX idx_events_received_at ON events (received_at);
CREATE INDEX idx_events_severity    ON events (severity);
