-- 029_alert_rules_timestamp_fix.sql
-- Fix created_at / updated_at / fired_at columns in alert_rules and
-- rule_executions from TEXT to TIMESTAMP so that mattn/go-sqlite3 can
-- scan them directly into time.Time via sqlx. Existing rows are preserved.

PRAGMA foreign_keys = OFF;

-- Recreate alert_rules with TIMESTAMP columns.
CREATE TABLE alert_rules_new (
  id               TEXT      PRIMARY KEY,
  name             TEXT      NOT NULL,
  enabled          INTEGER   NOT NULL DEFAULT 1,
  source_id        TEXT,
  source_type      TEXT,
  severity         TEXT,
  conditions       TEXT      NOT NULL DEFAULT '[]',
  condition_logic  TEXT      NOT NULL DEFAULT 'AND',
  delivery_email   INTEGER   NOT NULL DEFAULT 0,
  delivery_push    INTEGER   NOT NULL DEFAULT 0,
  delivery_webhook INTEGER   NOT NULL DEFAULT 0,
  webhook_url      TEXT,
  notif_title      TEXT      NOT NULL DEFAULT '',
  notif_body       TEXT      NOT NULL DEFAULT '',
  created_at       TIMESTAMP NOT NULL,
  updated_at       TIMESTAMP NOT NULL
);

INSERT INTO alert_rules_new SELECT * FROM alert_rules;
DROP TABLE alert_rules;
ALTER TABLE alert_rules_new RENAME TO alert_rules;

-- Recreate rule_executions with TIMESTAMP fired_at.
CREATE TABLE rule_executions_new (
  id       TEXT      PRIMARY KEY,
  rule_id  TEXT      NOT NULL,
  event_id TEXT      NOT NULL,
  fired_at TIMESTAMP NOT NULL,
  delivery TEXT      NOT NULL,
  success  INTEGER   NOT NULL,
  error    TEXT
);

INSERT INTO rule_executions_new SELECT * FROM rule_executions;
DROP TABLE rule_executions;
ALTER TABLE rule_executions_new RENAME TO rule_executions;

CREATE INDEX idx_rule_executions_rule_id  ON rule_executions (rule_id);
CREATE INDEX idx_rule_executions_fired_at ON rule_executions (fired_at);

PRAGMA foreign_keys = ON;
