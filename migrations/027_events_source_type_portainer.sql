-- 027_events_source_type_portainer.sql
-- Adds 'portainer' to the source_type CHECK constraint on the events table.
-- The original constraint was defined in 020_events_unified_schema.sql and only
-- listed the types known at that time. SQLite does not support ALTER COLUMN, so
-- the table must be recreated.

PRAGMA foreign_keys = OFF;

CREATE TABLE events_new (
    id          TEXT      PRIMARY KEY,
    level       TEXT      NOT NULL CHECK (level IN ('debug', 'info', 'warn', 'error', 'critical')),
    source_name TEXT      NOT NULL,
    source_type TEXT      NOT NULL CHECK (source_type IN (
                                'app',
                                'physical_host',
                                'virtual_host',
                                'docker_engine',
                                'monitor_check',
                                'system',
                                'portainer'
                            )),
    source_id   TEXT,
    title       TEXT      NOT NULL,
    payload     TEXT,
    created_at  TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT INTO events_new SELECT * FROM events;

DROP TABLE events;
ALTER TABLE events_new RENAME TO events;

CREATE INDEX idx_events_created_at  ON events (created_at);
CREATE INDEX idx_events_level       ON events (level);
CREATE INDEX idx_events_source_type ON events (source_type);
CREATE INDEX idx_events_source_id   ON events (source_id);

PRAGMA foreign_keys = ON;
