-- 007_settings.sql
-- Generic key/value settings table for user-configurable options
-- such as SMTP credentials and digest schedule.

CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- Seed default digest schedule
INSERT INTO settings (key, value) VALUES (
    'digest_schedule',
    '{"frequency":"monthly","day_of_week":1,"day_of_month":1,"send_hour":8}'
);
