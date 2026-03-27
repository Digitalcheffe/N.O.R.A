package repo

import (
	"context"
	"fmt"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// CustomProfileRepo defines operations for user-created custom app templates.
type CustomProfileRepo interface {
	List(ctx context.Context) ([]models.CustomProfile, error)
	Get(ctx context.Context, id string) (*models.CustomProfile, error)
	Create(ctx context.Context, p *models.CustomProfile) error
	Delete(ctx context.Context, id string) error
}

type sqliteCustomProfileRepo struct {
	db *sqlx.DB
}

// NewCustomProfileRepo returns a CustomProfileRepo backed by the given SQLite database.
// The underlying table is custom_app_templates (renamed from custom_profiles in migration 006).
func NewCustomProfileRepo(db *sqlx.DB) CustomProfileRepo {
	return &sqliteCustomProfileRepo{db: db}
}

func (r *sqliteCustomProfileRepo) List(ctx context.Context) ([]models.CustomProfile, error) {
	var profiles []models.CustomProfile
	err := r.db.SelectContext(ctx, &profiles,
		`SELECT id, name, yaml_content, created_at FROM custom_app_templates ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list custom profiles: %w", err)
	}
	return profiles, nil
}

func (r *sqliteCustomProfileRepo) Get(ctx context.Context, id string) (*models.CustomProfile, error) {
	var p models.CustomProfile
	err := r.db.GetContext(ctx, &p,
		`SELECT id, name, yaml_content, created_at FROM custom_app_templates WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("get custom profile: %w", err)
	}
	return &p, nil
}

func (r *sqliteCustomProfileRepo) Create(ctx context.Context, p *models.CustomProfile) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO custom_app_templates (id, name, yaml_content) VALUES (?, ?, ?)`,
		p.ID, p.Name, p.YAMLContent)
	if err != nil {
		return fmt.Errorf("create custom profile: %w", err)
	}
	return nil
}

func (r *sqliteCustomProfileRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM custom_app_templates WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete custom profile: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
