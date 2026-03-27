package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// ---- PhysicalHostRepo -------------------------------------------------------

// PhysicalHostRepo defines CRUD for physical_hosts.
type PhysicalHostRepo interface {
	List(ctx context.Context) ([]models.PhysicalHost, error)
	Create(ctx context.Context, h *models.PhysicalHost) error
	Get(ctx context.Context, id string) (*models.PhysicalHost, error)
	Update(ctx context.Context, h *models.PhysicalHost) error
	Delete(ctx context.Context, id string) error
}

type sqlitePhysicalHostRepo struct{ db *sqlx.DB }

// NewPhysicalHostRepo returns a PhysicalHostRepo backed by SQLite.
func NewPhysicalHostRepo(db *sqlx.DB) PhysicalHostRepo {
	return &sqlitePhysicalHostRepo{db: db}
}

func (r *sqlitePhysicalHostRepo) List(ctx context.Context) ([]models.PhysicalHost, error) {
	var hosts []models.PhysicalHost
	err := r.db.SelectContext(ctx, &hosts,
		`SELECT id, name, ip, type, COALESCE(notes,'') AS notes, created_at
		 FROM physical_hosts ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list physical_hosts: %w", err)
	}
	return hosts, nil
}

func (r *sqlitePhysicalHostRepo) Create(ctx context.Context, h *models.PhysicalHost) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO physical_hosts (id, name, ip, type, notes) VALUES (?, ?, ?, ?, NULLIF(?, ''))`,
		h.ID, h.Name, h.IP, h.Type, h.Notes)
	if err != nil {
		return fmt.Errorf("create physical_host: %w", err)
	}
	return nil
}

func (r *sqlitePhysicalHostRepo) Get(ctx context.Context, id string) (*models.PhysicalHost, error) {
	var h models.PhysicalHost
	err := r.db.GetContext(ctx, &h,
		`SELECT id, name, ip, type, COALESCE(notes,'') AS notes, created_at
		 FROM physical_hosts WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get physical_host: %w", err)
	}
	return &h, nil
}

func (r *sqlitePhysicalHostRepo) Update(ctx context.Context, h *models.PhysicalHost) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE physical_hosts SET name=?, ip=?, type=?, notes=NULLIF(?, '') WHERE id=?`,
		h.Name, h.IP, h.Type, h.Notes, h.ID)
	if err != nil {
		return fmt.Errorf("update physical_host: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqlitePhysicalHostRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM physical_hosts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete physical_host: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- VirtualHostRepo --------------------------------------------------------

// VirtualHostRepo defines CRUD for virtual_hosts.
type VirtualHostRepo interface {
	List(ctx context.Context) ([]models.VirtualHost, error)
	Create(ctx context.Context, h *models.VirtualHost) error
	Get(ctx context.Context, id string) (*models.VirtualHost, error)
	Update(ctx context.Context, h *models.VirtualHost) error
	Delete(ctx context.Context, id string) error
}

type sqliteVirtualHostRepo struct{ db *sqlx.DB }

// NewVirtualHostRepo returns a VirtualHostRepo backed by SQLite.
func NewVirtualHostRepo(db *sqlx.DB) VirtualHostRepo {
	return &sqliteVirtualHostRepo{db: db}
}

func (r *sqliteVirtualHostRepo) List(ctx context.Context) ([]models.VirtualHost, error) {
	var hosts []models.VirtualHost
	err := r.db.SelectContext(ctx, &hosts,
		`SELECT id, COALESCE(physical_host_id,'') AS physical_host_id, name, ip, type, created_at
		 FROM virtual_hosts ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list virtual_hosts: %w", err)
	}
	return hosts, nil
}

func (r *sqliteVirtualHostRepo) Create(ctx context.Context, h *models.VirtualHost) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO virtual_hosts (id, physical_host_id, name, ip, type) VALUES (?, NULLIF(?, ''), ?, ?, ?)`,
		h.ID, h.PhysicalHostID, h.Name, h.IP, h.Type)
	if err != nil {
		return fmt.Errorf("create virtual_host: %w", err)
	}
	return nil
}

func (r *sqliteVirtualHostRepo) Get(ctx context.Context, id string) (*models.VirtualHost, error) {
	var h models.VirtualHost
	err := r.db.GetContext(ctx, &h,
		`SELECT id, COALESCE(physical_host_id,'') AS physical_host_id, name, ip, type, created_at
		 FROM virtual_hosts WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get virtual_host: %w", err)
	}
	return &h, nil
}

func (r *sqliteVirtualHostRepo) Update(ctx context.Context, h *models.VirtualHost) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE virtual_hosts SET physical_host_id=NULLIF(?, ''), name=?, ip=?, type=? WHERE id=?`,
		h.PhysicalHostID, h.Name, h.IP, h.Type, h.ID)
	if err != nil {
		return fmt.Errorf("update virtual_host: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteVirtualHostRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM virtual_hosts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete virtual_host: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- DockerEngineRepo -------------------------------------------------------

// DockerEngineRepo defines CRUD for docker_engines.
type DockerEngineRepo interface {
	List(ctx context.Context) ([]models.DockerEngine, error)
	Create(ctx context.Context, e *models.DockerEngine) error
	Get(ctx context.Context, id string) (*models.DockerEngine, error)
	Update(ctx context.Context, e *models.DockerEngine) error
	Delete(ctx context.Context, id string) error
}

type sqliteDockerEngineRepo struct{ db *sqlx.DB }

// NewDockerEngineRepo returns a DockerEngineRepo backed by SQLite.
func NewDockerEngineRepo(db *sqlx.DB) DockerEngineRepo {
	return &sqliteDockerEngineRepo{db: db}
}

func (r *sqliteDockerEngineRepo) List(ctx context.Context) ([]models.DockerEngine, error) {
	var engines []models.DockerEngine
	err := r.db.SelectContext(ctx, &engines,
		`SELECT id, COALESCE(virtual_host_id,'') AS virtual_host_id, name, socket_type, socket_path, created_at
		 FROM docker_engines ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list docker_engines: %w", err)
	}
	return engines, nil
}

func (r *sqliteDockerEngineRepo) Create(ctx context.Context, e *models.DockerEngine) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO docker_engines (id, virtual_host_id, name, socket_type, socket_path) VALUES (?, NULLIF(?, ''), ?, ?, ?)`,
		e.ID, e.VirtualHostID, e.Name, e.SocketType, e.SocketPath)
	if err != nil {
		return fmt.Errorf("create docker_engine: %w", err)
	}
	return nil
}

func (r *sqliteDockerEngineRepo) Get(ctx context.Context, id string) (*models.DockerEngine, error) {
	var e models.DockerEngine
	err := r.db.GetContext(ctx, &e,
		`SELECT id, COALESCE(virtual_host_id,'') AS virtual_host_id, name, socket_type, socket_path, created_at
		 FROM docker_engines WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get docker_engine: %w", err)
	}
	return &e, nil
}

func (r *sqliteDockerEngineRepo) Update(ctx context.Context, e *models.DockerEngine) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE docker_engines SET virtual_host_id=NULLIF(?, ''), name=?, socket_type=?, socket_path=? WHERE id=?`,
		e.VirtualHostID, e.Name, e.SocketType, e.SocketPath, e.ID)
	if err != nil {
		return fmt.Errorf("update docker_engine: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteDockerEngineRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM docker_engines WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete docker_engine: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
