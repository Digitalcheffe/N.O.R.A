CREATE TABLE IF NOT EXISTS app_metric_snapshots (
    id          TEXT PRIMARY KEY,
    app_id      TEXT NOT NULL,
    profile_id  TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    label       TEXT NOT NULL,
    value       TEXT NOT NULL,
    value_type  TEXT NOT NULL,
    polled_at   DATETIME NOT NULL,

    UNIQUE(app_id, metric_name)
);

CREATE INDEX IF NOT EXISTS idx_metric_snapshots_app    ON app_metric_snapshots(app_id);
CREATE INDEX IF NOT EXISTS idx_metric_snapshots_polled ON app_metric_snapshots(polled_at);
