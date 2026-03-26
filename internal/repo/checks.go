package repo

import (
	"context"
	"fmt"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// CheckRepo defines read operations for the monitor_checks table.
type CheckRepo interface {
	List(ctx context.Context) ([]models.MonitorCheck, error)
}

type sqliteCheckRepo struct {
	db *sqlx.DB
}

// NewCheckRepo returns a CheckRepo backed by the given SQLite database.
func NewCheckRepo(db *sqlx.DB) CheckRepo {
	return &sqliteCheckRepo{db: db}
}

func (r *sqliteCheckRepo) List(ctx context.Context) ([]models.MonitorCheck, error) {
	var checks []models.MonitorCheck
	err := r.db.SelectContext(ctx, &checks, `
		SELECT id, COALESCE(app_id,'') AS app_id, name, type, target,
		       interval_secs, COALESCE(expected_status,0) AS expected_status,
		       ssl_warn_days, ssl_crit_days, enabled,
		       last_checked_at, COALESCE(last_status,'') AS last_status,
		       COALESCE(last_result,'') AS last_result, created_at
		FROM monitor_checks
		ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list checks: %w", err)
	}
	if checks == nil {
		checks = []models.MonitorCheck{}
	}
	return checks, nil
}
