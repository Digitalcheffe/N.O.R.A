package repo

import (
	"context"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// VirtualHostRepository defines data access for virtual hosts.
type VirtualHostRepository interface {
	Create(ctx context.Context, host *models.VirtualHost) error
	GetByID(ctx context.Context, id string) (*models.VirtualHost, error)
	List(ctx context.Context) ([]*models.VirtualHost, error)
	Update(ctx context.Context, host *models.VirtualHost) error
	Delete(ctx context.Context, id string) error
	ListByPhysicalHost(ctx context.Context, physicalHostID string) ([]*models.VirtualHost, error)
}

type sqliteVirtualHostRepo struct{ db *sqlx.DB }

func (r *sqliteVirtualHostRepo) Create(ctx context.Context, host *models.VirtualHost) error {
	host.ID = uuid.NewString()
	host.CreatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO virtual_hosts (id, physical_host_id, name, ip, type, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		host.ID, host.PhysicalHostID, host.Name, host.IP, host.Type, host.CreatedAt,
	)
	return err
}

func (r *sqliteVirtualHostRepo) GetByID(ctx context.Context, id string) (*models.VirtualHost, error) {
	var h models.VirtualHost
	err := r.db.GetContext(ctx, &h, `SELECT * FROM virtual_hosts WHERE id = ?`, id)
	return &h, err
}

func (r *sqliteVirtualHostRepo) List(ctx context.Context) ([]*models.VirtualHost, error) {
	var hosts []*models.VirtualHost
	err := r.db.SelectContext(ctx, &hosts, `SELECT * FROM virtual_hosts ORDER BY created_at`)
	return hosts, err
}

func (r *sqliteVirtualHostRepo) Update(ctx context.Context, host *models.VirtualHost) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE virtual_hosts SET physical_host_id = ?, name = ?, ip = ?, type = ? WHERE id = ?`,
		host.PhysicalHostID, host.Name, host.IP, host.Type, host.ID,
	)
	return err
}

func (r *sqliteVirtualHostRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM virtual_hosts WHERE id = ?`, id)
	return err
}

func (r *sqliteVirtualHostRepo) ListByPhysicalHost(
	ctx context.Context, physicalHostID string,
) ([]*models.VirtualHost, error) {
	var hosts []*models.VirtualHost
	err := r.db.SelectContext(ctx, &hosts,
		`SELECT * FROM virtual_hosts WHERE physical_host_id = ? ORDER BY created_at`,
		physicalHostID,
	)
	return hosts, err
}
