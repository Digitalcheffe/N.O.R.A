package repo

import (
	"context"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// MonitorCheckRepository defines data access for monitor checks.
type MonitorCheckRepository interface {
	Create(ctx context.Context, check *models.MonitorCheck) error
	GetByID(ctx context.Context, id string) (*models.MonitorCheck, error)
	List(ctx context.Context) ([]*models.MonitorCheck, error)
	Update(ctx context.Context, check *models.MonitorCheck) error
	Delete(ctx context.Context, id string) error
	ListEnabled(ctx context.Context) ([]*models.MonitorCheck, error)
	UpdateStatus(ctx context.Context, id string, checkedAt time.Time, status string, result string) error
}

type sqliteMonitorCheckRepo struct{ db *sqlx.DB }

func (r *sqliteMonitorCheckRepo) Create(ctx context.Context, check *models.MonitorCheck) error {
	check.ID = uuid.NewString()
	check.CreatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO monitor_checks
		 (id, app_id, name, type, target, interval_secs, expected_status,
		  ssl_warn_days, ssl_crit_days, enabled, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		check.ID, check.AppID, check.Name, check.Type, check.Target,
		check.IntervalSecs, check.ExpectedStatus,
		check.SSLWarnDays, check.SSLCritDays, check.Enabled, check.CreatedAt,
	)
	return err
}

func (r *sqliteMonitorCheckRepo) GetByID(ctx context.Context, id string) (*models.MonitorCheck, error) {
	var c models.MonitorCheck
	err := r.db.GetContext(ctx, &c, `SELECT * FROM monitor_checks WHERE id = ?`, id)
	return &c, err
}

func (r *sqliteMonitorCheckRepo) List(ctx context.Context) ([]*models.MonitorCheck, error) {
	var checks []*models.MonitorCheck
	err := r.db.SelectContext(ctx, &checks, `SELECT * FROM monitor_checks ORDER BY created_at`)
	return checks, err
}

func (r *sqliteMonitorCheckRepo) Update(ctx context.Context, check *models.MonitorCheck) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE monitor_checks
		 SET app_id = ?, name = ?, type = ?, target = ?, interval_secs = ?,
		     expected_status = ?, ssl_warn_days = ?, ssl_crit_days = ?, enabled = ?
		 WHERE id = ?`,
		check.AppID, check.Name, check.Type, check.Target, check.IntervalSecs,
		check.ExpectedStatus, check.SSLWarnDays, check.SSLCritDays, check.Enabled,
		check.ID,
	)
	return err
}

func (r *sqliteMonitorCheckRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM monitor_checks WHERE id = ?`, id)
	return err
}

func (r *sqliteMonitorCheckRepo) ListEnabled(ctx context.Context) ([]*models.MonitorCheck, error) {
	var checks []*models.MonitorCheck
	err := r.db.SelectContext(ctx, &checks,
		`SELECT * FROM monitor_checks WHERE enabled = 1 ORDER BY created_at`)
	return checks, err
}

func (r *sqliteMonitorCheckRepo) UpdateStatus(
	ctx context.Context, id string, checkedAt time.Time, status string, result string,
) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE monitor_checks SET last_checked_at = ?, last_status = ?, last_result = ? WHERE id = ?`,
		checkedAt.UTC(), status, result, id,
	)
	return err
}
