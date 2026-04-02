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
	GetByEmail(ctx context.Context, email string) (*models.User, string, error)
	Count(ctx context.Context) (int, error)
	// UpdatePassword replaces the stored password hash for the given user ID.
	UpdatePassword(ctx context.Context, id string, newHash string) error
	// UpdateUser updates the email and role for the given user ID.
	UpdateUser(ctx context.Context, id string, email string, role string) error

	// TOTP methods
	// GetTOTPData returns the TOTP secret and flags for the given user.
	GetTOTPData(ctx context.Context, id string) (secret string, enabled bool, grace bool, exempt bool, err error)
	// SetTOTPSecret stores a pending (not yet confirmed) TOTP secret.
	SetTOTPSecret(ctx context.Context, id string, secret string) error
	// EnableTOTP marks TOTP as confirmed/active and clears grace.
	EnableTOTP(ctx context.Context, id string) error
	// DisableTOTP clears the TOTP secret, disables TOTP, and restores grace to 1.
	DisableTOTP(ctx context.Context, id string) error
	// ClearGrace sets totp_grace = 0 (called after a grace login).
	ClearGrace(ctx context.Context, id string) error
	// ResetGrace sets totp_grace = 1 (admin action: gives a user one more grace login).
	ResetGrace(ctx context.Context, id string) error
	// SetTOTPExempt sets the totp_exempt flag (first admin is exempt from global MFA).
	SetTOTPExempt(ctx context.Context, id string, exempt bool) error
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
		SELECT id, email, role, created_at, totp_enabled, totp_grace, totp_exempt
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
		SELECT id, email, role, created_at, totp_enabled, totp_grace, totp_exempt
		FROM users WHERE id = ?`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &u, nil
}

// GetByEmail returns the user and their stored password hash for the given email.
func (r *sqliteUserRepo) GetByEmail(ctx context.Context, email string) (*models.User, string, error) {
	var row struct {
		models.User
		PasswordHash string `db:"password_hash"`
	}
	err := r.db.GetContext(ctx, &row, `
		SELECT id, email, role, created_at, totp_enabled, totp_grace, totp_exempt, password_hash
		FROM users WHERE email = ?`, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", ErrNotFound
		}
		return nil, "", fmt.Errorf("get user by email: %w", err)
	}
	return &row.User, row.PasswordHash, nil
}

// UpdatePassword replaces the stored password hash for the given user ID.
func (r *sqliteUserRepo) UpdatePassword(ctx context.Context, id string, newHash string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, newHash, id)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateUser updates the email and role for the given user ID.
func (r *sqliteUserRepo) UpdateUser(ctx context.Context, id string, email string, role string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE users SET email = ?, role = ? WHERE id = ?`, email, role, id)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Count returns the total number of users.
func (r *sqliteUserRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM users`)
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

// GetTOTPData returns the TOTP fields for the given user ID.
func (r *sqliteUserRepo) GetTOTPData(ctx context.Context, id string) (secret string, enabled bool, grace bool, exempt bool, err error) {
	var row struct {
		Secret  sql.NullString `db:"totp_secret"`
		Enabled bool           `db:"totp_enabled"`
		Grace   bool           `db:"totp_grace"`
		Exempt  bool           `db:"totp_exempt"`
	}
	if err = r.db.GetContext(ctx, &row, `
		SELECT totp_secret, totp_enabled, totp_grace, totp_exempt
		FROM users WHERE id = ?`, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = ErrNotFound
		}
		return
	}
	secret = row.Secret.String
	enabled = row.Enabled
	grace = row.Grace
	exempt = row.Exempt
	return
}

// SetTOTPSecret stores a pending TOTP secret without enabling TOTP.
func (r *sqliteUserRepo) SetTOTPSecret(ctx context.Context, id string, secret string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET totp_secret = ? WHERE id = ?`, secret, id)
	return err
}

// EnableTOTP marks TOTP as active and clears the grace flag.
func (r *sqliteUserRepo) EnableTOTP(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET totp_enabled = 1, totp_grace = 0 WHERE id = ?`, id)
	return err
}

// DisableTOTP clears the TOTP secret, disables TOTP, and resets grace to 1.
func (r *sqliteUserRepo) DisableTOTP(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET totp_secret = NULL, totp_enabled = 0, totp_grace = 1 WHERE id = ?`, id)
	return err
}

// ClearGrace sets totp_grace = 0 after a grace login is consumed.
func (r *sqliteUserRepo) ClearGrace(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET totp_grace = 0 WHERE id = ?`, id)
	return err
}

// ResetGrace sets totp_grace = 1 (admin grants one more grace login).
func (r *sqliteUserRepo) ResetGrace(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET totp_grace = 1 WHERE id = ?`, id)
	return err
}

// SetTOTPExempt sets or clears the totp_exempt flag for the given user.
func (r *sqliteUserRepo) SetTOTPExempt(ctx context.Context, id string, exempt bool) error {
	val := 0
	if exempt {
		val = 1
	}
	_, err := r.db.ExecContext(ctx, `UPDATE users SET totp_exempt = ? WHERE id = ?`, val, id)
	return err
}
