package repo

import (
	"context"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// RollupRepository defines data access for monthly event rollups.
type RollupRepository interface {
	Upsert(ctx context.Context, rollup *models.Rollup) error
	GetByAppAndPeriod(ctx context.Context, appID string, year int, month int) ([]*models.Rollup, error)
}

type sqliteRollupRepo struct{ db *sqlx.DB }

func (r *sqliteRollupRepo) Upsert(ctx context.Context, rollup *models.Rollup) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO rollups (app_id, year, month, event_type, severity, count)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(app_id, year, month, event_type, severity)
		 DO UPDATE SET count = excluded.count`,
		rollup.AppID, rollup.Year, rollup.Month, rollup.EventType, rollup.Severity, rollup.Count,
	)
	return err
}

func (r *sqliteRollupRepo) GetByAppAndPeriod(
	ctx context.Context, appID string, year int, month int,
) ([]*models.Rollup, error) {
	var rollups []*models.Rollup
	err := r.db.SelectContext(ctx, &rollups,
		`SELECT * FROM rollups WHERE app_id = ? AND year = ? AND month = ?`,
		appID, year, month,
	)
	return rollups, err
}
