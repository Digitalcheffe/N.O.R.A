package repo

import (
	"context"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// AppRepository defines data access for apps.
type AppRepository interface {
	Create(ctx context.Context, app *models.App) error
	GetByID(ctx context.Context, id string) (*models.App, error)
	GetByToken(ctx context.Context, token string) (*models.App, error)
	List(ctx context.Context) ([]*models.App, error)
	Update(ctx context.Context, app *models.App) error
	Delete(ctx context.Context, id string) error
}

type sqliteAppRepo struct{ db *sqlx.DB }

func (r *sqliteAppRepo) Create(ctx context.Context, app *models.App) error {
	app.ID = uuid.NewString()
	app.CreatedAt = time.Now().UTC()
	if app.Config == nil {
		app.Config = models.RawMessage("{}")
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO apps (id, name, token, profile_id, docker_engine_id, config, rate_limit, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		app.ID, app.Name, app.Token, app.ProfileID, app.DockerEngineID,
		app.Config, app.RateLimit, app.CreatedAt,
	)
	return err
}

func (r *sqliteAppRepo) GetByID(ctx context.Context, id string) (*models.App, error) {
	var a models.App
	err := r.db.GetContext(ctx, &a, `SELECT * FROM apps WHERE id = ?`, id)
	return &a, err
}

func (r *sqliteAppRepo) GetByToken(ctx context.Context, token string) (*models.App, error) {
	var a models.App
	err := r.db.GetContext(ctx, &a, `SELECT * FROM apps WHERE token = ?`, token)
	return &a, err
}

func (r *sqliteAppRepo) List(ctx context.Context) ([]*models.App, error) {
	var apps []*models.App
	err := r.db.SelectContext(ctx, &apps, `SELECT * FROM apps ORDER BY created_at`)
	return apps, err
}

func (r *sqliteAppRepo) Update(ctx context.Context, app *models.App) error {
	if app.Config == nil {
		app.Config = models.RawMessage("{}")
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE apps SET name = ?, token = ?, profile_id = ?, docker_engine_id = ?, config = ?, rate_limit = ?
		 WHERE id = ?`,
		app.Name, app.Token, app.ProfileID, app.DockerEngineID,
		app.Config, app.RateLimit, app.ID,
	)
	return err
}

func (r *sqliteAppRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM apps WHERE id = ?`, id)
	return err
}
