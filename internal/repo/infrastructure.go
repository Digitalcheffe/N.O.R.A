package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// ── InfraComponentRepo ───────────────────────────────────────────────────────

// InfraComponentRepo defines CRUD for infrastructure_components.
type InfraComponentRepo interface {
	List(ctx context.Context) ([]models.InfrastructureComponent, error)
	// ListByParent returns all components that are direct children of parentID
	// in component_links, ordered by type then name.
	ListByParent(ctx context.Context, parentID string) ([]models.InfrastructureComponent, error)
	Get(ctx context.Context, id string) (*models.InfrastructureComponent, error)
	Create(ctx context.Context, c *models.InfrastructureComponent) error
	Update(ctx context.Context, c *models.InfrastructureComponent) error
	Delete(ctx context.Context, id string) error
	// UpdateStatus sets last_polled_at and last_status without touching other fields.
	UpdateStatus(ctx context.Context, id, status, lastPolledAt string) error
	// UpdateIP sets the ip field on a child component discovered by Proxmox.
	// Only the ip column is touched; all other fields are unchanged.
	UpdateIP(ctx context.Context, id, ip string) error
	// UpdateMeta stores the latest poller snapshot JSON on the component
	// without touching any other fields.
	UpdateMeta(ctx context.Context, id, metaJSON string) error
}

type sqliteInfraComponentRepo struct{ db *sqlx.DB }

// NewInfraComponentRepo returns an InfraComponentRepo backed by SQLite.
func NewInfraComponentRepo(db *sqlx.DB) InfraComponentRepo {
	return &sqliteInfraComponentRepo{db: db}
}

func (r *sqliteInfraComponentRepo) List(ctx context.Context) ([]models.InfrastructureComponent, error) {
	var rows []models.InfrastructureComponent
	err := r.db.SelectContext(ctx, &rows, `
		SELECT id, name, COALESCE(ip,'') AS ip, type, collection_method,
		       credentials, snmp_config, meta,
		       COALESCE(notes,'') AS notes, enabled,
		       last_polled_at, COALESCE(last_status,'unknown') AS last_status,
		       created_at
		FROM infrastructure_components
		ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list infrastructure_components: %w", err)
	}
	if rows == nil {
		rows = []models.InfrastructureComponent{}
	}
	return rows, nil
}

func (r *sqliteInfraComponentRepo) ListByParent(ctx context.Context, parentID string) ([]models.InfrastructureComponent, error) {
	var rows []models.InfrastructureComponent
	err := r.db.SelectContext(ctx, &rows, `
		SELECT ic.id, ic.name, COALESCE(ic.ip,'') AS ip, ic.type, ic.collection_method,
		       ic.credentials, ic.snmp_config, ic.meta,
		       COALESCE(ic.notes,'') AS notes, ic.enabled,
		       ic.last_polled_at, COALESCE(ic.last_status,'unknown') AS last_status,
		       ic.created_at
		FROM infrastructure_components ic
		INNER JOIN component_links cl ON cl.child_id = ic.id
		WHERE cl.parent_id = ?
		ORDER BY ic.type ASC, ic.name ASC`, parentID)
	if err != nil {
		return nil, fmt.Errorf("list children for %s: %w", parentID, err)
	}
	if rows == nil {
		rows = []models.InfrastructureComponent{}
	}
	return rows, nil
}

func (r *sqliteInfraComponentRepo) Get(ctx context.Context, id string) (*models.InfrastructureComponent, error) {
	var c models.InfrastructureComponent
	err := r.db.GetContext(ctx, &c, `
		SELECT id, name, COALESCE(ip,'') AS ip, type, collection_method,
		       credentials, snmp_config, meta,
		       COALESCE(notes,'') AS notes, enabled,
		       last_polled_at, COALESCE(last_status,'unknown') AS last_status,
		       created_at
		FROM infrastructure_components WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get infrastructure_component: %w", err)
	}
	return &c, nil
}

func (r *sqliteInfraComponentRepo) Create(ctx context.Context, c *models.InfrastructureComponent) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO infrastructure_components
		  (id, name, ip, type, collection_method, credentials, snmp_config, notes, enabled, last_status, created_at)
		VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?)`,
		c.ID, c.Name, c.IP, c.Type, c.CollectionMethod,
		c.Credentials, c.SNMPConfig,
		c.Notes, c.Enabled, c.LastStatus, c.CreatedAt)
	if err != nil {
		return fmt.Errorf("create infrastructure_component: %w", err)
	}
	return nil
}

func (r *sqliteInfraComponentRepo) Update(ctx context.Context, c *models.InfrastructureComponent) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE infrastructure_components
		SET name=?, ip=NULLIF(?, ''), type=?, collection_method=?,
		    credentials=?, snmp_config=?,
		    notes=NULLIF(?, ''), enabled=?, last_status=?
		WHERE id=?`,
		c.Name, c.IP, c.Type, c.CollectionMethod,
		c.Credentials, c.SNMPConfig,
		c.Notes, c.Enabled, c.LastStatus, c.ID)
	if err != nil {
		return fmt.Errorf("update infrastructure_component: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteInfraComponentRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM infrastructure_components WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete infrastructure_component: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteInfraComponentRepo) UpdateStatus(ctx context.Context, id, status, lastPolledAt string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE infrastructure_components
		SET last_status = ?, last_polled_at = ?
		WHERE id = ?`,
		status, lastPolledAt, id)
	if err != nil {
		return fmt.Errorf("update component status: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteInfraComponentRepo) UpdateIP(ctx context.Context, id, ip string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE infrastructure_components SET ip = ? WHERE id = ?`,
		ip, id)
	if err != nil {
		return fmt.Errorf("update component ip: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteInfraComponentRepo) UpdateMeta(ctx context.Context, id, metaJSON string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE infrastructure_components SET meta = ? WHERE id = ?`,
		metaJSON, id)
	if err != nil {
		return fmt.Errorf("update meta: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

