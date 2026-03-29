package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// AppRepo defines CRUD operations for apps.
type AppRepo interface {
	List(ctx context.Context) ([]models.App, error)
	// ListByHost returns all apps whose host_component_id matches hostID.
	ListByHost(ctx context.Context, hostID string) ([]models.App, error)
	Create(ctx context.Context, app *models.App) error
	Get(ctx context.Context, id string) (*models.App, error)
	GetByToken(ctx context.Context, token string) (*models.App, error)
	Update(ctx context.Context, app *models.App) error
	Delete(ctx context.Context, id string) error
	UpdateToken(ctx context.Context, id, token string) error
	// SetDockerEngineID sets the docker_engine_id on the app unconditionally.
	SetDockerEngineID(ctx context.Context, appID, engineID string) error
	// SetHostComponentID links or unlinks an app to an infrastructure component.
	SetHostComponentID(ctx context.Context, appID string, hostID *string) error
}

type sqliteAppRepo struct {
	db *sqlx.DB
}

// NewAppRepo returns an AppRepo backed by the given SQLite database.
func NewAppRepo(db *sqlx.DB) AppRepo {
	return &sqliteAppRepo{db: db}
}

func (r *sqliteAppRepo) List(ctx context.Context) ([]models.App, error) {
	var apps []models.App
	err := r.db.SelectContext(ctx, &apps, `
		SELECT id, name, token, COALESCE(profile_id,'') AS profile_id,
		       COALESCE(docker_engine_id,'') AS docker_engine_id,
		       host_component_id,
		       config, rate_limit, created_at
		FROM apps ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	return apps, nil
}

func (r *sqliteAppRepo) ListByHost(ctx context.Context, hostID string) ([]models.App, error) {
	var apps []models.App
	err := r.db.SelectContext(ctx, &apps, `
		SELECT id, name, token, COALESCE(profile_id,'') AS profile_id,
		       COALESCE(docker_engine_id,'') AS docker_engine_id,
		       host_component_id,
		       config, rate_limit, created_at
		FROM apps WHERE host_component_id = ? ORDER BY created_at ASC`, hostID)
	if err != nil {
		return nil, fmt.Errorf("list apps by host: %w", err)
	}
	if apps == nil {
		apps = []models.App{}
	}
	return apps, nil
}

func (r *sqliteAppRepo) Create(ctx context.Context, app *models.App) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO apps (id, name, token, profile_id, docker_engine_id, host_component_id, config, rate_limit)
		VALUES (?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?)`,
		app.ID, app.Name, app.Token, app.ProfileID, app.DockerEngineID,
		app.HostComponentID, app.Config, app.RateLimit)
	if err != nil {
		return fmt.Errorf("create app: %w", err)
	}
	return nil
}

func (r *sqliteAppRepo) Get(ctx context.Context, id string) (*models.App, error) {
	var app models.App
	err := r.db.GetContext(ctx, &app, `
		SELECT id, name, token, COALESCE(profile_id,'') AS profile_id,
		       COALESCE(docker_engine_id,'') AS docker_engine_id,
		       host_component_id,
		       config, rate_limit, created_at
		FROM apps WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}
	return &app, nil
}

func (r *sqliteAppRepo) GetByToken(ctx context.Context, token string) (*models.App, error) {
	var app models.App
	err := r.db.GetContext(ctx, &app, `
		SELECT id, name, token, COALESCE(profile_id,'') AS profile_id,
		       COALESCE(docker_engine_id,'') AS docker_engine_id,
		       host_component_id,
		       config, rate_limit, created_at
		FROM apps WHERE token = ?`, token)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get app by token: %w", err)
	}
	return &app, nil
}

func (r *sqliteAppRepo) Update(ctx context.Context, app *models.App) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE apps SET name=?, profile_id=NULLIF(?,?), docker_engine_id=NULLIF(?,?),
		    host_component_id=?, config=?, rate_limit=?
		WHERE id=?`,
		app.Name, app.ProfileID, "", app.DockerEngineID, "",
		app.HostComponentID, app.Config, app.RateLimit, app.ID)
	if err != nil {
		return fmt.Errorf("update app: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteAppRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM apps WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete app: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteAppRepo) UpdateToken(ctx context.Context, id, token string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE apps SET token=? WHERE id=?`, token, id)
	if err != nil {
		return fmt.Errorf("update token: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteAppRepo) SetDockerEngineID(ctx context.Context, appID, engineID string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE apps SET docker_engine_id=? WHERE id=?`, engineID, appID)
	if err != nil {
		return fmt.Errorf("set docker_engine_id on app %s: %w", appID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteAppRepo) SetHostComponentID(ctx context.Context, appID string, hostID *string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE apps SET host_component_id=? WHERE id=?`, hostID, appID)
	if err != nil {
		return fmt.Errorf("set host_component_id on app %s: %w", appID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
