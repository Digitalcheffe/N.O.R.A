package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// AppTopEntry is a per-app summary row used in instance-wide metrics.
type AppTopEntry struct {
	AppID         string `db:"app_id"`
	AppName       string `db:"app_name"`
	EventsPerHour int    `db:"events_per_hour"`
}

// MetricsRepo defines write and read operations for the metrics table.
type MetricsRepo interface {
	Upsert(ctx context.Context, m *models.Metric) error
	// ListByApp returns the most recent `limit` metric records for the given app,
	// ordered newest-first.
	ListByApp(ctx context.Context, appID string, limit int) ([]models.Metric, error)
	// TopApps returns the top `limit` apps by most-recent events_per_hour.
	TopApps(ctx context.Context, limit int) ([]AppTopEntry, error)
}

type sqliteMetricsRepo struct {
	db *sqlx.DB
}

// NewMetricsRepo returns a MetricsRepo backed by the given SQLite database.
func NewMetricsRepo(db *sqlx.DB) MetricsRepo {
	return &sqliteMetricsRepo{db: db}
}

func (r *sqliteMetricsRepo) Upsert(ctx context.Context, m *models.Metric) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO metrics (app_id, period, events_per_hour, avg_payload_bytes, peak_per_minute)
		VALUES (?, ?, ?, ?, ?)`,
		m.AppID,
		m.Period.UTC().Format(time.RFC3339),
		m.EventsPerHour,
		m.AvgPayloadBytes,
		m.PeakPerMinute,
	)
	if err != nil {
		return fmt.Errorf("upsert metric: %w", err)
	}
	return nil
}

func (r *sqliteMetricsRepo) ListByApp(ctx context.Context, appID string, limit int) ([]models.Metric, error) {
	if limit <= 0 {
		limit = 24
	}
	var rows []models.Metric
	err := r.db.SelectContext(ctx, &rows, `
		SELECT app_id, period, events_per_hour, avg_payload_bytes, peak_per_minute
		FROM metrics
		WHERE app_id = ?
		ORDER BY period DESC
		LIMIT ?`,
		appID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list metrics by app: %w", err)
	}
	if rows == nil {
		rows = []models.Metric{}
	}
	return rows, nil
}

func (r *sqliteMetricsRepo) TopApps(ctx context.Context, limit int) ([]AppTopEntry, error) {
	if limit <= 0 {
		limit = 10
	}
	var rows []AppTopEntry
	err := r.db.SelectContext(ctx, &rows, `
		SELECT m.app_id, COALESCE(a.name, m.app_id) AS app_name, m.events_per_hour
		FROM metrics m
		JOIN apps a ON a.id = m.app_id
		WHERE m.period = (
			SELECT MAX(m2.period) FROM metrics m2 WHERE m2.app_id = m.app_id
		)
		ORDER BY m.events_per_hour DESC
		LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("top apps: %w", err)
	}
	if rows == nil {
		rows = []AppTopEntry{}
	}
	return rows, nil
}
