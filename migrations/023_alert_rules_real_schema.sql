-- 023_alert_rules_real_schema.sql
-- Replace the v2 stub alert_rules table (T-03) with the full T-32 schema.
-- Adds rule_executions for delivery audit logging.
-- Retains: rule_executions rows for 30 days (handled by retention job).

DROP TABLE IF EXISTS alert_rules;

CREATE TABLE alert_rules (
  id               TEXT PRIMARY KEY,
  name             TEXT NOT NULL,
  enabled          INTEGER NOT NULL DEFAULT 1,
  source_id        TEXT,            -- FK apps.id, NULL = any source
  source_type      TEXT,            -- 'app' | 'docker' | 'monitor' | NULL = any
  severity         TEXT,            -- specific level or NULL = any severity
  conditions       TEXT NOT NULL DEFAULT '[]',   -- JSON array of {field, operator, value}
  condition_logic  TEXT NOT NULL DEFAULT 'AND',  -- 'AND' | 'OR'
  delivery_email   INTEGER NOT NULL DEFAULT 0,
  delivery_push    INTEGER NOT NULL DEFAULT 0,
  delivery_webhook INTEGER NOT NULL DEFAULT 0,
  webhook_url      TEXT,
  notif_title      TEXT NOT NULL DEFAULT '',
  notif_body       TEXT NOT NULL DEFAULT '',
  created_at       TEXT NOT NULL,
  updated_at       TEXT NOT NULL
);

CREATE TABLE rule_executions (
  id          TEXT PRIMARY KEY,
  rule_id     TEXT NOT NULL,
  event_id    TEXT NOT NULL,
  fired_at    TEXT NOT NULL,
  delivery    TEXT NOT NULL,   -- 'email' | 'push' | 'webhook'
  success     INTEGER NOT NULL,
  error       TEXT
);

CREATE INDEX idx_rule_executions_rule_id  ON rule_executions (rule_id);
CREATE INDEX idx_rule_executions_fired_at ON rule_executions (fired_at);
