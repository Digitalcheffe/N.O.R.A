package repo

import (
	"context"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// MetricsRepository defines data access for app-level metrics.
type MetricsRepository interface {
	Upsert(ctx context.Context, metric *models.Metric) error
	GetByApp(ctx context.Context, appID string) ([]*models.Metric, error)
}

type sqliteMetricsRepo struct{ db *sqlx.DB }

func (r *sqliteMetricsRepo) Upsert(ctx context.Context, metric *models.Metric) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO metrics (app_id, period, events_per_hour, avg_payload_bytes, peak_per_minute)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(app_id, period)
		 DO UPDATE SET
		     events_per_hour   = excluded.events_per_hour,
		     avg_payload_bytes = excluded.avg_payload_bytes,
		     peak_per_minute   = excluded.peak_per_minute`,
		metric.AppID, metric.Period.UTC(),
		metric.EventsPerHour, metric.AvgPayloadBytes, metric.PeakPerMinute,
	)
	return err
}

func (r *sqliteMetricsRepo) GetByApp(ctx context.Context, appID string) ([]*models.Metric, error) {
	var metrics []*models.Metric
	err := r.db.SelectContext(ctx, &metrics,
		`SELECT * FROM metrics WHERE app_id = ? ORDER BY period DESC`, appID)
	return metrics, err
}
