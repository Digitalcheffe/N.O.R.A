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

	// LatestMetrics returns the most recent value of cpu_percent and mem_percent
	// for each sourceID in sourceIDs, filtered by sourceType.
	// Returns map[sourceID]map[metric]float64. Missing entries mean no data yet.
	LatestMetrics(ctx context.Context, sourceType string, sourceIDs []string) (map[string]map[string]float64, error)

	// BackfillAppID sets app_id on all resource_readings for the given docker
	// containerID (source_type='docker_container') where app_id is currently NULL.
	// Returns the number of rows updated.
	BackfillAppID(ctx context.Context, containerID, appID string) (int64, error)
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

func (r *sqliteResourceReadingRepo) LatestMetrics(ctx context.Context, sourceType string, sourceIDs []string) (map[string]map[string]float64, error) {
	result := make(map[string]map[string]float64)
	if len(sourceIDs) == 0 {
		return result, nil
	}

	query, args, err := sqlx.In(`
		WITH latest AS (
			SELECT source_id, metric, value,
			       ROW_NUMBER() OVER (PARTITION BY source_id, metric ORDER BY recorded_at DESC) AS rn
			FROM resource_readings
			WHERE source_type = ?
			  AND source_id   IN (?)
			  AND metric      IN ('cpu_percent', 'mem_percent')
		)
		SELECT source_id, metric, value FROM latest WHERE rn = 1
	`, append([]interface{}{sourceType}, toStringInterfaces(sourceIDs)...)...)
	if err != nil {
		return nil, fmt.Errorf("build latest metrics query: %w", err)
	}
	query = r.db.Rebind(query)

	rows, err := r.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("latest metrics: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var sourceID, metric string
		var value float64
		if err := rows.Scan(&sourceID, &metric, &value); err != nil {
			return nil, fmt.Errorf("scan latest metrics row: %w", err)
		}
		if result[sourceID] == nil {
			result[sourceID] = make(map[string]float64)
		}
		result[sourceID][metric] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("latest metrics rows: %w", err)
	}
	return result, nil
}

func (r *sqliteResourceReadingRepo) BackfillAppID(ctx context.Context, containerID, appID string) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE resource_readings SET app_id = ? WHERE source_type = 'docker_container' AND source_id = ? AND app_id IS NULL`,
		appID, containerID)
	if err != nil {
		return 0, fmt.Errorf("backfill resource readings app_id: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// toStringInterfaces converts a []string to []interface{} for use with sqlx.In.
func toStringInterfaces(ss []string) []interface{} {
	out := make([]interface{}, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
