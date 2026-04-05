CREATE TABLE IF NOT EXISTS digest_registry (
    id          TEXT PRIMARY KEY,
    profile_id  TEXT NOT NULL,
    source      TEXT NOT NULL CHECK(source IN ('webhook', 'api')),
    entry_type  TEXT NOT NULL CHECK(entry_type IN ('category', 'widget')),
    name        TEXT NOT NULL,
    label       TEXT NOT NULL,
    config      TEXT NOT NULL DEFAULT '{}',
    active      INTEGER NOT NULL DEFAULT 1,
    created_at  TIMESTAMP NOT NULL,
    updated_at  TIMESTAMP NOT NULL,

    UNIQUE(profile_id, name)
);

CREATE INDEX IF NOT EXISTS idx_digest_registry_profile ON digest_registry(profile_id);
CREATE INDEX IF NOT EXISTS idx_digest_registry_active  ON digest_registry(active);
