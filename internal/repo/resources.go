package repo

import (
	"context"
	"fmt"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// ResourceReadingRepo defines write operations for the resource_readings table.
type ResourceReadingRepo interface {
	// Create persists a single resource metric reading.
	Create(ctx context.Context, r *models.ResourceReading) error
}

type sqliteResourceReadingRepo struct {
	db *sqlx.DB
}

// NewResourceReadingRepo returns a ResourceReadingRepo backed by the given SQLite database.
func NewResourceReadingRepo(db *sqlx.DB) ResourceReadingRepo {
	return &sqliteResourceReadingRepo{db: db}
}

func (r *sqliteResourceReadingRepo) Create(ctx context.Context, reading *models.ResourceReading) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO resource_readings (id, source_id, source_type, metric, value, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		reading.ID, reading.SourceID, reading.SourceType, reading.Metric, reading.Value, reading.RecordedAt)
	if err != nil {
		return fmt.Errorf("create resource reading: %w", err)
	}
	return nil
}
