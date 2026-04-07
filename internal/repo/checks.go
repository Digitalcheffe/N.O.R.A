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
	// ListBySourceComponent returns all checks owned by the given component.
	ListBySourceComponent(ctx context.Context, componentID string) ([]models.MonitorCheck, error)
	// DeleteBySourceComponent removes all checks owned by the given component.
	DeleteBySourceComponent(ctx context.Context, componentID string) error
	// UpsertForComponent inserts or updates a Traefik-owned SSL check.
	UpsertForComponent(ctx context.Context, check *models.MonitorCheck) error
	// ExistsForTypeAndTarget reports whether any check with the given type and target exists.
	ExistsForTypeAndTarget(ctx context.Context, checkType, target string) (bool, error)
	// SetDNSBaseline stores the captured baseline value for a DNS check.
	SetDNSBaseline(ctx context.Context, id, baseline string) error
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
	       ssl_warn_days, ssl_crit_days, ssl_source, integration_id,
	       source_component_id,
	       COALESCE(skip_tls_verify,0) AS skip_tls_verify,
	       COALESCE(dns_record_type,'') AS dns_record_type,
	       COALESCE(dns_expected_value,'') AS dns_expected_value,
	       COALESCE(dns_resolver,'') AS dns_resolver,
	       enabled,
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
		   ssl_warn_days, ssl_crit_days, ssl_source, integration_id, skip_tls_verify,
		   dns_record_type, dns_expected_value, dns_resolver, enabled)
		VALUES (?, NULLIF(?,''), ?, ?, ?, ?, NULLIF(?,0), ?, ?, ?, ?, ?, NULLIF(?,''), NULLIF(?,''), NULLIF(?,''), ?)`,
		check.ID, check.AppID,
		check.Name, check.Type, check.Target, check.IntervalSecs,
		check.ExpectedStatus,
		check.SSLWarnDays, check.SSLCritDays,
		check.SSLSource, check.IntegrationID,
		check.SkipTLSVerify,
		check.DNSRecordType,
		check.DNSExpectedValue,
		check.DNSResolver,
		check.Enabled)
	if err != nil {
		return fmt.Errorf("create check: %w", err)
	}
	// Keep component_links in sync: monitor → app.
	if check.AppID != "" {
		r.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO component_links (parent_type, parent_id, child_type, child_id, created_at)
			VALUES ('monitor', ?, 'app', ?, datetime('now'))`,
			check.ID, check.AppID)
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
		SET app_id=NULLIF(?,''), name=?, type=?, target=?, interval_secs=?,
		    expected_status=NULLIF(?,0), ssl_warn_days=?, ssl_crit_days=?,
		    ssl_source=?, integration_id=?, skip_tls_verify=?,
		    dns_record_type=NULLIF(?,''), dns_expected_value=NULLIF(?,''),
		    dns_resolver=NULLIF(?,''),
		    enabled=?
		WHERE id=?`,
		check.AppID,
		check.Name, check.Type, check.Target, check.IntervalSecs,
		check.ExpectedStatus,
		check.SSLWarnDays, check.SSLCritDays,
		check.SSLSource, check.IntegrationID,
		check.SkipTLSVerify,
		check.DNSRecordType,
		check.DNSExpectedValue,
		check.DNSResolver,
		check.Enabled,
		check.ID)
	if err != nil {
		return fmt.Errorf("update check: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	// Keep component_links in sync: monitor → app.
	// Always remove the old link first, then re-insert if app_id is still set.
	r.db.ExecContext(ctx, `
		DELETE FROM component_links WHERE parent_type = 'monitor' AND parent_id = ?`, check.ID)
	if check.AppID != "" {
		r.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO component_links (parent_type, parent_id, child_type, child_id, created_at)
			VALUES ('monitor', ?, 'app', ?, datetime('now'))`,
			check.ID, check.AppID)
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

func (r *sqliteCheckRepo) ListBySourceComponent(ctx context.Context, componentID string) ([]models.MonitorCheck, error) {
	var checks []models.MonitorCheck
	err := r.db.SelectContext(ctx, &checks, selectCheckCols+` WHERE source_component_id = ? ORDER BY created_at ASC`, componentID)
	if err != nil {
		return nil, fmt.Errorf("list checks by component: %w", err)
	}
	if checks == nil {
		checks = []models.MonitorCheck{}
	}
	return checks, nil
}

func (r *sqliteCheckRepo) DeleteBySourceComponent(ctx context.Context, componentID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM monitor_checks WHERE source_component_id = ?`, componentID)
	if err != nil {
		return fmt.Errorf("delete checks by component: %w", err)
	}
	return nil
}

func (r *sqliteCheckRepo) ExistsForTypeAndTarget(ctx context.Context, checkType, target string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM monitor_checks WHERE type = ? AND target = ?`, checkType, target).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("exists check for type/target: %w", err)
	}
	return count > 0, nil
}

func (r *sqliteCheckRepo) SetDNSBaseline(ctx context.Context, id, baseline string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE monitor_checks SET dns_expected_value = ? WHERE id = ?`, baseline, id)
	if err != nil {
		return fmt.Errorf("set dns baseline: %w", err)
	}
	return nil
}

// UpsertForComponent inserts or updates a Traefik-owned SSL check identified by
// (source_component_id, target). The id field on check must be pre-populated;
// on conflict the existing row's id is preserved.
func (r *sqliteCheckRepo) UpsertForComponent(ctx context.Context, check *models.MonitorCheck) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO monitor_checks
		  (id, app_id, name, type, target, interval_secs, expected_status,
		   ssl_warn_days, ssl_crit_days, ssl_source, integration_id,
		   source_component_id, skip_tls_verify, enabled)
		VALUES (?, NULLIF(?,''), ?, ?, ?, ?, NULLIF(?,0), ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  name               = excluded.name,
		  ssl_warn_days      = excluded.ssl_warn_days,
		  ssl_crit_days      = excluded.ssl_crit_days,
		  ssl_source         = excluded.ssl_source,
		  source_component_id = excluded.source_component_id,
		  enabled            = excluded.enabled`,
		check.ID, check.AppID,
		check.Name, check.Type, check.Target, check.IntervalSecs,
		check.ExpectedStatus,
		check.SSLWarnDays, check.SSLCritDays,
		check.SSLSource, check.IntegrationID,
		check.SourceComponentID,
		check.SkipTLSVerify,
		check.Enabled)
	if err != nil {
		return fmt.Errorf("upsert check for component: %w", err)
	}
	// Keep component_links in sync: monitor → app.
	if check.AppID != "" {
		r.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO component_links (parent_type, parent_id, child_type, child_id, created_at)
			VALUES ('monitor', ?, 'app', ?, datetime('now'))`,
			check.ID, check.AppID)
	}
	return nil
}
