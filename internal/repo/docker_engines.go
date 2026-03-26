package repo

import (
	"context"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// DockerEngineRepository defines data access for Docker engines.
type DockerEngineRepository interface {
	Create(ctx context.Context, engine *models.DockerEngine) error
	GetByID(ctx context.Context, id string) (*models.DockerEngine, error)
	List(ctx context.Context) ([]*models.DockerEngine, error)
	Update(ctx context.Context, engine *models.DockerEngine) error
	Delete(ctx context.Context, id string) error
	ListByVirtualHost(ctx context.Context, virtualHostID string) ([]*models.DockerEngine, error)
}

type sqliteDockerEngineRepo struct{ db *sqlx.DB }

func (r *sqliteDockerEngineRepo) Create(ctx context.Context, engine *models.DockerEngine) error {
	engine.ID = uuid.NewString()
	engine.CreatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO docker_engines (id, virtual_host_id, name, socket_type, socket_path, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		engine.ID, engine.VirtualHostID, engine.Name,
		engine.SocketType, engine.SocketPath, engine.CreatedAt,
	)
	return err
}

func (r *sqliteDockerEngineRepo) GetByID(ctx context.Context, id string) (*models.DockerEngine, error) {
	var e models.DockerEngine
	err := r.db.GetContext(ctx, &e, `SELECT * FROM docker_engines WHERE id = ?`, id)
	return &e, err
}

func (r *sqliteDockerEngineRepo) List(ctx context.Context) ([]*models.DockerEngine, error) {
	var engines []*models.DockerEngine
	err := r.db.SelectContext(ctx, &engines, `SELECT * FROM docker_engines ORDER BY created_at`)
	return engines, err
}

func (r *sqliteDockerEngineRepo) Update(ctx context.Context, engine *models.DockerEngine) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE docker_engines
		 SET virtual_host_id = ?, name = ?, socket_type = ?, socket_path = ?
		 WHERE id = ?`,
		engine.VirtualHostID, engine.Name, engine.SocketType, engine.SocketPath, engine.ID,
	)
	return err
}

func (r *sqliteDockerEngineRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM docker_engines WHERE id = ?`, id)
	return err
}

func (r *sqliteDockerEngineRepo) ListByVirtualHost(
	ctx context.Context, virtualHostID string,
) ([]*models.DockerEngine, error) {
	var engines []*models.DockerEngine
	err := r.db.SelectContext(ctx, &engines,
		`SELECT * FROM docker_engines WHERE virtual_host_id = ? ORDER BY created_at`,
		virtualHostID,
	)
	return engines, err
}
