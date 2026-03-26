package repo

import (
	"context"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// ResourceReadingRepository defines data access for raw resource readings.
type ResourceReadingRepository interface {
	Create(ctx context.Context, reading *models.ResourceReading) error
	ListBySource(ctx context.Context, sourceID string) ([]*models.ResourceReading, error)
	DeleteOlderThan(ctx context.Context, cutoff time.Time) error
}

type sqliteResourceReadingRepo struct{ db *sqlx.DB }

func (r *sqliteResourceReadingRepo) Create(ctx context.Context, reading *models.ResourceReading) error {
	reading.ID = uuid.NewString()
	reading.RecordedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO resource_readings (id, source_id, source_type, metric, value, recorded_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		reading.ID, reading.SourceID, reading.SourceType,
		reading.Metric, reading.Value, reading.RecordedAt,
	)
	return err
}

func (r *sqliteResourceReadingRepo) ListBySource(
	ctx context.Context, sourceID string,
) ([]*models.ResourceReading, error) {
	var readings []*models.ResourceReading
	err := r.db.SelectContext(ctx, &readings,
		`SELECT * FROM resource_readings WHERE source_id = ? ORDER BY recorded_at DESC`,
		sourceID,
	)
	return readings, err
}

func (r *sqliteResourceReadingRepo) DeleteOlderThan(ctx context.Context, cutoff time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM resource_readings WHERE recorded_at < ?`, cutoff.UTC())
	return err
}
