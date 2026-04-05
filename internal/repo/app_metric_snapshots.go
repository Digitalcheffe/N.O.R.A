package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// AppMetricSnapshotRepo defines operations for app_metric_snapshots.
type AppMetricSnapshotRepo interface {
	// Upsert inserts or replaces the snapshot using INSERT OR REPLACE on the
	// UNIQUE(app_id, metric_name) constraint. Always keeps the latest value only.
	Upsert(ctx context.Context, snapshot models.AppMetricSnapshot) error
	// ListByApp returns all snapshots for the given app_id.
	ListByApp(ctx context.Context, appID string) ([]models.AppMetricSnapshot, error)
	// GetByAppAndMetric returns the single snapshot for (app_id, metric_name),
	// or ErrNotFound if none exists.
	GetByAppAndMetric(ctx context.Context, appID string, metricName string) (*models.AppMetricSnapshot, error)
}

type sqliteAppMetricSnapshotRepo struct {
	db *sqlx.DB
}

// NewAppMetricSnapshotRepo returns an AppMetricSnapshotRepo backed by db.
func NewAppMetricSnapshotRepo(db *sqlx.DB) AppMetricSnapshotRepo {
	return &sqliteAppMetricSnapshotRepo{db: db}
}

func (r *sqliteAppMetricSnapshotRepo) Upsert(ctx context.Context, s models.AppMetricSnapshot) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO app_metric_snapshots
		    (id, app_id, profile_id, metric_name, label, value, value_type, polled_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.AppID, s.ProfileID, s.MetricName, s.Label, s.Value, s.ValueType,
		s.PolledAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("upsert app metric snapshot: %w", err)
	}
	return nil
}

func (r *sqliteAppMetricSnapshotRepo) ListByApp(ctx context.Context, appID string) ([]models.AppMetricSnapshot, error) {
	var rows []models.AppMetricSnapshot
	err := r.db.SelectContext(ctx, &rows, `
		SELECT id, app_id, profile_id, metric_name, label, value, value_type, polled_at
		FROM app_metric_snapshots WHERE app_id = ? ORDER BY metric_name ASC`, appID)
	if err != nil {
		return nil, fmt.Errorf("list app metric snapshots: %w", err)
	}
	if rows == nil {
		rows = []models.AppMetricSnapshot{}
	}
	return rows, nil
}

func (r *sqliteAppMetricSnapshotRepo) GetByAppAndMetric(ctx context.Context, appID string, metricName string) (*models.AppMetricSnapshot, error) {
	var s models.AppMetricSnapshot
	err := r.db.GetContext(ctx, &s, `
		SELECT id, app_id, profile_id, metric_name, label, value, value_type, polled_at
		FROM app_metric_snapshots WHERE app_id = ? AND metric_name = ?`, appID, metricName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get app metric snapshot: %w", err)
	}
	return &s, nil
}
