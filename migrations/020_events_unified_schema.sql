-- 020_events_unified_schema.sql
-- Unify the events table to support all event origins: app webhooks, infra
-- pollers, monitor checks, Docker watchers, and internal system activity.
--
-- Field renames:
--   severity     → level
--   received_at  → created_at
--   display_text → title
--   raw_payload  → payload (merged with fields, nullable)
--   app_id       → source_id (generalized FK, no constraint)
--
-- New fields:
--   source_name  TEXT NOT NULL  — human-readable entity name
--   source_type  TEXT NOT NULL  — app | physical_host | virtual_host |
--                                  docker_engine | monitor_check | system
--
-- Severity → level mapping for app profile data (documented here):
--   critical → critical
--   high     → error
--   medium   → warn
--   low      → info
--   info     → info
--   debug    → debug
-- Existing events already use debug/info/warn/error/critical so migration is 1:1.
--
-- Payload strategy: existing raw_payload and fields are merged using json_patch
-- so that normalized extracted fields (e.g. event_type) remain queryable via
-- json_extract(payload, '$.event_type'). This preserves rollup queries.

CREATE TABLE events_new (
    id          TEXT      PRIMARY KEY,
    level       TEXT      NOT NULL CHECK (level IN ('debug', 'info', 'warn', 'error', 'critical')),
    source_name TEXT      NOT NULL,
    source_type TEXT      NOT NULL CHECK (source_type IN ('app', 'physical_host', 'virtual_host', 'docker_engine', 'monitor_check', 'system')),
    source_id   TEXT,
    title       TEXT      NOT NULL,
    payload     TEXT,
    created_at  TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- Migrate existing rows. Source classification:
--   app_id IS NOT NULL              → source_type = 'app'
--   fields contains check_id       → source_type = 'monitor_check', source_id from fields
--   all others                     → source_type = 'system'
--
-- source_name:
--   app events: joined app name (fallback 'Unknown App')
--   monitor_check events: json_extract(fields, '$.check_name') or display_text
--   system events: component_name from fields or 'NORA System'
--
-- payload: json_patch merges fields onto raw_payload so extracted keys like
--   event_type become top-level queryable fields.
INSERT INTO events_new (id, level, source_name, source_type, source_id, title, payload, created_at)
SELECT
    e.id,
    e.severity,
    CASE
        WHEN e.app_id IS NOT NULL
            THEN COALESCE((SELECT a.name FROM apps a WHERE a.id = e.app_id), 'Unknown App')
        WHEN json_extract(e.fields, '$.check_id') IS NOT NULL
            THEN COALESCE(e.display_text, 'Monitor Check')
        WHEN json_extract(e.fields, '$.component_name') IS NOT NULL
            THEN json_extract(e.fields, '$.component_name')
        ELSE 'NORA System'
    END,
    CASE
        WHEN e.app_id IS NOT NULL                            THEN 'app'
        WHEN json_extract(e.fields, '$.check_id') IS NOT NULL THEN 'monitor_check'
        ELSE 'system'
    END,
    CASE
        WHEN e.app_id IS NOT NULL
            THEN e.app_id
        WHEN json_extract(e.fields, '$.check_id') IS NOT NULL
            THEN json_extract(e.fields, '$.check_id')
        WHEN json_extract(e.fields, '$.component_id') IS NOT NULL
            THEN json_extract(e.fields, '$.component_id')
        ELSE NULL
    END,
    e.display_text,
    CASE
        WHEN e.raw_payload = '{}' AND e.fields = '{}'   THEN NULL
        WHEN e.raw_payload = '{}' AND e.fields != '{}'  THEN e.fields
        WHEN e.raw_payload != '{}' AND e.fields = '{}'  THEN e.raw_payload
        ELSE json_patch(e.raw_payload, e.fields)
    END,
    e.received_at
FROM events e;

DROP TABLE events;
ALTER TABLE events_new RENAME TO events;

CREATE INDEX idx_events_created_at  ON events (created_at);
CREATE INDEX idx_events_level       ON events (level);
CREATE INDEX idx_events_source_type ON events (source_type);
CREATE INDEX idx_events_source_id   ON events (source_id);
