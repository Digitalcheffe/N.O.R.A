package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// MetricsRepo defines write operations for the metrics table.
type MetricsRepo interface {
	Upsert(ctx context.Context, m *models.Metric) error
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
