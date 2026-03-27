package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// UserRepo defines read/write operations for the users table.
type UserRepo interface {
	List(ctx context.Context) ([]models.User, error)
	// Create inserts a new user. passwordHash must be a pre-hashed value.
	Create(ctx context.Context, u *models.User, passwordHash string) error
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*models.User, error)
}

type sqliteUserRepo struct {
	db *sqlx.DB
}

// NewUserRepo returns a UserRepo backed by the given SQLite database.
func NewUserRepo(db *sqlx.DB) UserRepo {
	return &sqliteUserRepo{db: db}
}

func (r *sqliteUserRepo) List(ctx context.Context) ([]models.User, error) {
	var users []models.User
	err := r.db.SelectContext(ctx, &users, `
		SELECT id, email, role, created_at
		FROM users ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	if users == nil {
		users = []models.User{}
	}
	return users, nil
}

func (r *sqliteUserRepo) Create(ctx context.Context, u *models.User, passwordHash string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, role)
		VALUES (?, ?, ?, ?)`,
		u.ID, u.Email, passwordHash, u.Role)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *sqliteUserRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteUserRepo) GetByID(ctx context.Context, id string) (*models.User, error) {
	var u models.User
	err := r.db.GetContext(ctx, &u, `
		SELECT id, email, role, created_at FROM users WHERE id = ?`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &u, nil
}
