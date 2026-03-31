package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// SnapshotRepo defines storage for slowly-changing condition snapshots.
type SnapshotRepo interface {
	// GetLatest returns the most recently captured snapshot for the given
	// (entityID, metricKey) pair. Returns ErrNotFound if no reading exists yet.
	GetLatest(ctx context.Context, entityID, metricKey string) (*models.Snapshot, error)
	// Insert writes a new snapshot row.
	Insert(ctx context.Context, s *models.Snapshot) error
	// Prune deletes all but the most recent limit rows for (entityID, metricKey).
	Prune(ctx context.Context, entityID, metricKey string, limit int) error
}

type sqliteSnapshotRepo struct{ db *sqlx.DB }

// NewSnapshotRepo returns a SnapshotRepo backed by SQLite.
func NewSnapshotRepo(db *sqlx.DB) SnapshotRepo {
	return &sqliteSnapshotRepo{db: db}
}

func (r *sqliteSnapshotRepo) GetLatest(ctx context.Context, entityID, metricKey string) (*models.Snapshot, error) {
	var s models.Snapshot
	err := r.db.GetContext(ctx, &s, `
		SELECT id, entity_type, entity_id, metric_key, metric_value, previous_value, captured_at
		FROM snapshots
		WHERE entity_id = ? AND metric_key = ?
		ORDER BY captured_at DESC
		LIMIT 1`, entityID, metricKey)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get latest snapshot: %w", err)
	}
	return &s, nil
}

func (r *sqliteSnapshotRepo) Insert(ctx context.Context, s *models.Snapshot) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO snapshots
		  (id, entity_type, entity_id, metric_key, metric_value, previous_value, captured_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.EntityType, s.EntityID, s.MetricKey, s.MetricValue, s.PreviousValue, s.CapturedAt)
	if err != nil {
		return fmt.Errorf("insert snapshot: %w", err)
	}
	return nil
}

func (r *sqliteSnapshotRepo) Prune(ctx context.Context, entityID, metricKey string, limit int) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM snapshots
		WHERE entity_id = ? AND metric_key = ?
		  AND id NOT IN (
		    SELECT id FROM snapshots
		    WHERE entity_id = ? AND metric_key = ?
		    ORDER BY captured_at DESC
		    LIMIT ?
		  )`, entityID, metricKey, entityID, metricKey, limit)
	if err != nil {
		return fmt.Errorf("prune snapshots %s/%s: %w", entityID, metricKey, err)
	}
	return nil
}
