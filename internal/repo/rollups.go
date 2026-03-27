package repo

import (
	"context"
	"fmt"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)


// RollupRepo defines read/write operations for the rollups table.
type RollupRepo interface {
	ListByPeriod(ctx context.Context, year, month int) ([]models.Rollup, error)
	// Upsert inserts or replaces a rollup row. Safe to call multiple times.
	Upsert(ctx context.Context, rollup *models.Rollup) error
}

type sqliteRollupRepo struct {
	db *sqlx.DB
}

// NewRollupRepo returns a RollupRepo backed by the given SQLite database.
func NewRollupRepo(db *sqlx.DB) RollupRepo {
	return &sqliteRollupRepo{db: db}
}

func (r *sqliteRollupRepo) Upsert(ctx context.Context, rollup *models.Rollup) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO rollups (app_id, year, month, event_type, severity, count)
		VALUES (?, ?, ?, ?, ?, ?)`,
		rollup.AppID, rollup.Year, rollup.Month, rollup.EventType, rollup.Severity, rollup.Count)
	if err != nil {
		return fmt.Errorf("upsert rollup: %w", err)
	}
	return nil
}

func (r *sqliteRollupRepo) ListByPeriod(ctx context.Context, year, month int) ([]models.Rollup, error) {
	var rollups []models.Rollup
	err := r.db.SelectContext(ctx, &rollups, `
		SELECT app_id, year, month, event_type, severity, count
		FROM rollups
		WHERE year = ? AND month = ?
		ORDER BY app_id, event_type, severity`, year, month)
	if err != nil {
		return nil, fmt.Errorf("list rollups: %w", err)
	}
	if rollups == nil {
		rollups = []models.Rollup{}
	}
	return rollups, nil
}
