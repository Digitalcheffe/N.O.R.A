package repo

import (
	"context"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// PhysicalHostRepository defines data access for physical hosts.
type PhysicalHostRepository interface {
	Create(ctx context.Context, host *models.PhysicalHost) error
	GetByID(ctx context.Context, id string) (*models.PhysicalHost, error)
	List(ctx context.Context) ([]*models.PhysicalHost, error)
	Update(ctx context.Context, host *models.PhysicalHost) error
	Delete(ctx context.Context, id string) error
}

type sqlitePhysicalHostRepo struct{ db *sqlx.DB }

func (r *sqlitePhysicalHostRepo) Create(ctx context.Context, host *models.PhysicalHost) error {
	host.ID = uuid.NewString()
	host.CreatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO physical_hosts (id, name, ip, type, notes, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		host.ID, host.Name, host.IP, host.Type, host.Notes, host.CreatedAt,
	)
	return err
}

func (r *sqlitePhysicalHostRepo) GetByID(ctx context.Context, id string) (*models.PhysicalHost, error) {
	var h models.PhysicalHost
	err := r.db.GetContext(ctx, &h, `SELECT * FROM physical_hosts WHERE id = ?`, id)
	return &h, err
}

func (r *sqlitePhysicalHostRepo) List(ctx context.Context) ([]*models.PhysicalHost, error) {
	var hosts []*models.PhysicalHost
	err := r.db.SelectContext(ctx, &hosts, `SELECT * FROM physical_hosts ORDER BY created_at`)
	return hosts, err
}

func (r *sqlitePhysicalHostRepo) Update(ctx context.Context, host *models.PhysicalHost) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE physical_hosts SET name = ?, ip = ?, type = ?, notes = ? WHERE id = ?`,
		host.Name, host.IP, host.Type, host.Notes, host.ID,
	)
	return err
}

func (r *sqlitePhysicalHostRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM physical_hosts WHERE id = ?`, id)
	return err
}
