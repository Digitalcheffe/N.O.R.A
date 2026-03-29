package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

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
		`SELECT id, COALESCE(infra_component_id,'') AS infra_component_id, name, socket_type, socket_path, created_at
		 FROM docker_engines ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list docker_engines: %w", err)
	}
	return engines, nil
}

func (r *sqliteDockerEngineRepo) Create(ctx context.Context, e *models.DockerEngine) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO docker_engines (id, infra_component_id, name, socket_type, socket_path) VALUES (?, NULLIF(?, ''), ?, ?, ?)`,
		e.ID, e.InfraComponentID, e.Name, e.SocketType, e.SocketPath)
	if err != nil {
		return fmt.Errorf("create docker_engine: %w", err)
	}
	return nil
}

func (r *sqliteDockerEngineRepo) Get(ctx context.Context, id string) (*models.DockerEngine, error) {
	var e models.DockerEngine
	err := r.db.GetContext(ctx, &e,
		`SELECT id, COALESCE(infra_component_id,'') AS infra_component_id, name, socket_type, socket_path, created_at
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
		`UPDATE docker_engines SET infra_component_id=NULLIF(?, ''), name=?, socket_type=?, socket_path=? WHERE id=?`,
		e.InfraComponentID, e.Name, e.SocketType, e.SocketPath, e.ID)
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
