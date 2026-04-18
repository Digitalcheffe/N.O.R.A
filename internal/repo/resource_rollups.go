package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// ResourceAggregate holds computed avg/min/max for a single source+metric combination.
type ResourceAggregate struct {
	SourceID   string  `db:"source_id"`
	SourceType string  `db:"source_type"`
	Metric     string  `db:"metric"`
	Avg        float64 `db:"avg_val"`
	Min        float64 `db:"min_val"`
	Max        float64 `db:"max_val"`
}

// ResourceRollupRepo provides rollup aggregation, upsert, and purge operations.
type ResourceRollupRepo interface {
	// AggregateReadings returns avg/min/max per source+metric for raw readings in [from, to).
	AggregateReadings(ctx context.Context, from, to time.Time) ([]ResourceAggregate, error)
	// AggregateHourlyRollups returns avg/min/max per source+metric from hourly rollups in [from, to).
	AggregateHourlyRollups(ctx context.Context, from, to time.Time) ([]ResourceAggregate, error)
	// Upsert inserts or updates a resource_rollup row.
	Upsert(ctx context.Context, r *models.ResourceRollup) error
	// PurgeReadings deletes resource_readings with recorded_at < cutoff. Returns rows deleted.
	PurgeReadings(ctx context.Context, cutoff time.Time) (int64, error)
	// PurgeHourlyRollups deletes hourly resource_rollups with period_start < cutoff. Returns rows deleted.
	PurgeHourlyRollups(ctx context.Context, cutoff time.Time) (int64, error)
	// LatestForSource returns the most recent rollup row per metric for the given source and period type.
	LatestForSource(ctx context.Context, sourceID, sourceType, periodType string) ([]models.ResourceRollup, error)
	// LatestForSourcePrefix is like LatestForSource but also matches rows whose
	// source_id starts with sourceID+"/" — used for components (e.g. proxmox_node)
	// that write per-node readings with a compound source_id.
	LatestForSourcePrefix(ctx context.Context, sourceID, sourceType, periodType string) ([]models.ResourceRollup, error)
}

type sqliteResourceRollupRepo struct {
	db *sqlx.DB
}

// NewResourceRollupRepo returns a ResourceRollupRepo backed by the given SQLite database.
func NewResourceRollupRepo(db *sqlx.DB) ResourceRollupRepo {
	return &sqliteResourceRollupRepo{db: db}
}

func (r *sqliteResourceRollupRepo) AggregateReadings(ctx context.Context, from, to time.Time) ([]ResourceAggregate, error) {
	var rows []ResourceAggregate
	err := r.db.SelectContext(ctx, &rows, `
		SELECT source_id, source_type, metric,
		       AVG(value) AS avg_val,
		       MIN(value) AS min_val,
		       MAX(value) AS max_val
		FROM resource_readings
		WHERE recorded_at >= ? AND recorded_at < ?
		GROUP BY source_id, source_type, metric`,
		from, to)
	if err != nil {
		return nil, fmt.Errorf("aggregate readings: %w", err)
	}
	return rows, nil
}

func (r *sqliteResourceRollupRepo) AggregateHourlyRollups(ctx context.Context, from, to time.Time) ([]ResourceAggregate, error) {
	var rows []ResourceAggregate
	err := r.db.SelectContext(ctx, &rows, `
		SELECT source_id, source_type, metric,
		       AVG(avg) AS avg_val,
		       MIN(min) AS min_val,
		       MAX(max) AS max_val
		FROM resource_rollups
		WHERE period_type = 'hour' AND period_start >= ? AND period_start < ?
		GROUP BY source_id, source_type, metric`,
		from, to)
	if err != nil {
		return nil, fmt.Errorf("aggregate hourly rollups: %w", err)
	}
	return rows, nil
}

func (r *sqliteResourceRollupRepo) Upsert(ctx context.Context, rollup *models.ResourceRollup) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO resource_rollups (source_id, source_type, metric, period_type, period_start, avg, min, max)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (source_id, metric, period_type, period_start)
		DO UPDATE SET
		    source_type = excluded.source_type,
		    avg         = excluded.avg,
		    min         = excluded.min,
		    max         = excluded.max`,
		rollup.SourceID, rollup.SourceType, rollup.Metric,
		rollup.PeriodType, rollup.PeriodStart,
		rollup.Avg, rollup.Min, rollup.Max)
	if err != nil {
		return fmt.Errorf("upsert resource rollup: %w", err)
	}
	return nil
}

func (r *sqliteResourceRollupRepo) PurgeReadings(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM resource_readings WHERE recorded_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("purge readings: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *sqliteResourceRollupRepo) PurgeHourlyRollups(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM resource_rollups WHERE period_type = 'hour' AND period_start < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("purge hourly rollups: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *sqliteResourceRollupRepo) LatestForSource(ctx context.Context, sourceID, sourceType, periodType string) ([]models.ResourceRollup, error) {
	var rows []models.ResourceRollup
	err := r.db.SelectContext(ctx, &rows, `
		SELECT source_id, source_type, metric, period_type, period_start, avg, min, max
		FROM resource_rollups r1
		WHERE source_id = ? AND source_type = ? AND period_type = ?
		  AND period_start = (
		      SELECT MAX(period_start) FROM resource_rollups r2
		      WHERE r2.source_id = r1.source_id
		        AND r2.metric = r1.metric
		        AND r2.period_type = r1.period_type
		  )`,
		sourceID, sourceType, periodType)
	if err != nil {
		return nil, fmt.Errorf("latest for source: %w", err)
	}
	return rows, nil
}

func (r *sqliteResourceRollupRepo) LatestForSourcePrefix(ctx context.Context, sourceID, sourceType, periodType string) ([]models.ResourceRollup, error) {
	var rows []models.ResourceRollup
	prefix := sourceID + "/%"
	err := r.db.SelectContext(ctx, &rows, `
		SELECT source_id, source_type, metric, period_type, period_start,
		       AVG(avg) AS avg, MIN(min) AS min, MAX(max) AS max
		FROM resource_rollups r1
		WHERE (source_id = ? OR source_id LIKE ?) AND source_type = ? AND period_type = ?
		  AND period_start = (
		      SELECT MAX(period_start) FROM resource_rollups r2
		      WHERE (r2.source_id = ? OR r2.source_id LIKE ?)
		        AND r2.metric = r1.metric
		        AND r2.period_type = r1.period_type
		  )
		GROUP BY metric`,
		sourceID, prefix, sourceType, periodType, sourceID, prefix)
	if err != nil {
		return nil, fmt.Errorf("latest for source prefix: %w", err)
	}
	return rows, nil
}

