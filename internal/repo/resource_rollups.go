package repo

import (
	"context"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// ResourceRollupRepository defines data access for resource metric rollups.
type ResourceRollupRepository interface {
	Upsert(ctx context.Context, rollup *models.ResourceRollup) error
	GetBySource(ctx context.Context, sourceID string, metric string) ([]*models.ResourceRollup, error)
}

type sqliteResourceRollupRepo struct{ db *sqlx.DB }

func (r *sqliteResourceRollupRepo) Upsert(ctx context.Context, rollup *models.ResourceRollup) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO resource_rollups
		 (source_id, source_type, metric, period_type, period_start, avg, min, max)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(source_id, metric, period_type, period_start)
		 DO UPDATE SET
		     avg = excluded.avg,
		     min = excluded.min,
		     max = excluded.max`,
		rollup.SourceID, rollup.SourceType, rollup.Metric, rollup.PeriodType,
		rollup.PeriodStart.UTC(), rollup.Avg, rollup.Min, rollup.Max,
	)
	return err
}

func (r *sqliteResourceRollupRepo) GetBySource(
	ctx context.Context, sourceID string, metric string,
) ([]*models.ResourceRollup, error) {
	var rollups []*models.ResourceRollup
	err := r.db.SelectContext(ctx, &rollups,
		`SELECT * FROM resource_rollups WHERE source_id = ? AND metric = ? ORDER BY period_start DESC`,
		sourceID, metric,
	)
	return rollups, err
}
