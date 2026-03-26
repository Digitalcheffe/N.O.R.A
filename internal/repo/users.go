package repo

import (
	"context"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// UserRepository defines data access for users.
type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	GetByID(ctx context.Context, id string) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	List(ctx context.Context) ([]*models.User, error)
	Update(ctx context.Context, user *models.User) error
	Delete(ctx context.Context, id string) error
}

type sqliteUserRepo struct{ db *sqlx.DB }

func (r *sqliteUserRepo) Create(ctx context.Context, user *models.User) error {
	user.ID = uuid.NewString()
	user.CreatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, role, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		user.ID, user.Email, user.PasswordHash, user.Role, user.CreatedAt,
	)
	return err
}

func (r *sqliteUserRepo) GetByID(ctx context.Context, id string) (*models.User, error) {
	var u models.User
	err := r.db.GetContext(ctx, &u, `SELECT * FROM users WHERE id = ?`, id)
	return &u, err
}

func (r *sqliteUserRepo) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	err := r.db.GetContext(ctx, &u, `SELECT * FROM users WHERE email = ?`, email)
	return &u, err
}

func (r *sqliteUserRepo) List(ctx context.Context) ([]*models.User, error) {
	var users []*models.User
	err := r.db.SelectContext(ctx, &users, `SELECT * FROM users ORDER BY created_at`)
	return users, err
}

func (r *sqliteUserRepo) Update(ctx context.Context, user *models.User) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET email = ?, password_hash = ?, role = ? WHERE id = ?`,
		user.Email, user.PasswordHash, user.Role, user.ID,
	)
	return err
}

func (r *sqliteUserRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}
