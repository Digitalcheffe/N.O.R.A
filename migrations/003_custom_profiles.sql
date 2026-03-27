-- 003_custom_profiles.sql
-- Stores user-created custom app profiles edited via the in-browser YAML editor.

CREATE TABLE custom_profiles (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    yaml_content TEXT NOT NULL,
    created_at   TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
