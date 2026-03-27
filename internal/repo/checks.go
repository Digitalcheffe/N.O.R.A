package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// CheckRepo defines CRUD operations for the monitor_checks table.
type CheckRepo interface {
	List(ctx context.Context) ([]models.MonitorCheck, error)
	Create(ctx context.Context, check *models.MonitorCheck) error
	Get(ctx context.Context, id string) (*models.MonitorCheck, error)
	Update(ctx context.Context, check *models.MonitorCheck) error
	Delete(ctx context.Context, id string) error
	UpdateStatus(ctx context.Context, id, status, result string, checkedAt time.Time) error
}

type sqliteCheckRepo struct {
	db *sqlx.DB
}

// NewCheckRepo returns a CheckRepo backed by the given SQLite database.
func NewCheckRepo(db *sqlx.DB) CheckRepo {
	return &sqliteCheckRepo{db: db}
}

const selectCheckCols = `
	SELECT id, COALESCE(app_id,'') AS app_id, name, type, target,
	       interval_secs, COALESCE(expected_status,0) AS expected_status,
	       ssl_warn_days, ssl_crit_days, enabled,
	       last_checked_at, COALESCE(last_status,'') AS last_status,
	       COALESCE(last_result,'') AS last_result, created_at
	FROM monitor_checks`

func (r *sqliteCheckRepo) List(ctx context.Context) ([]models.MonitorCheck, error) {
	var checks []models.MonitorCheck
	err := r.db.SelectContext(ctx, &checks, selectCheckCols+` ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list checks: %w", err)
	}
	if checks == nil {
		checks = []models.MonitorCheck{}
	}
	return checks, nil
}

func (r *sqliteCheckRepo) Create(ctx context.Context, check *models.MonitorCheck) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO monitor_checks
		  (id, app_id, name, type, target, interval_secs, expected_status,
		   ssl_warn_days, ssl_crit_days, enabled)
		VALUES (?, NULLIF(?,?), ?, ?, ?, ?, NULLIF(?,0), ?, ?, ?)`,
		check.ID, check.AppID, check.AppID,
		check.Name, check.Type, check.Target, check.IntervalSecs,
		check.ExpectedStatus,
		check.SSLWarnDays, check.SSLCritDays, check.Enabled)
	if err != nil {
		return fmt.Errorf("create check: %w", err)
	}
	return nil
}

func (r *sqliteCheckRepo) Get(ctx context.Context, id string) (*models.MonitorCheck, error) {
	var check models.MonitorCheck
	err := r.db.GetContext(ctx, &check, selectCheckCols+` WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get check: %w", err)
	}
	return &check, nil
}

func (r *sqliteCheckRepo) Update(ctx context.Context, check *models.MonitorCheck) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE monitor_checks
		SET app_id=NULLIF(?,?), name=?, type=?, target=?, interval_secs=?,
		    expected_status=NULLIF(?,0), ssl_warn_days=?, ssl_crit_days=?, enabled=?
		WHERE id=?`,
		check.AppID, check.AppID,
		check.Name, check.Type, check.Target, check.IntervalSecs,
		check.ExpectedStatus,
		check.SSLWarnDays, check.SSLCritDays, check.Enabled,
		check.ID)
	if err != nil {
		return fmt.Errorf("update check: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteCheckRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM monitor_checks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete check: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteCheckRepo) UpdateStatus(ctx context.Context, id, status, result string, checkedAt time.Time) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE monitor_checks
		SET last_status=?, last_result=?, last_checked_at=?
		WHERE id=?`,
		status, result, checkedAt.UTC(), id)
	if err != nil {
		return fmt.Errorf("update check status: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
