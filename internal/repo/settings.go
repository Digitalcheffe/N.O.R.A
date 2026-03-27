package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// SettingsRepo provides typed get/set access to the settings table.
type SettingsRepo interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	GetJSON(ctx context.Context, key string, out any) error
	SetJSON(ctx context.Context, key string, v any) error
}

type sqliteSettingsRepo struct {
	db *sqlx.DB
}

// NewSettingsRepo returns a SettingsRepo backed by the given SQLite database.
func NewSettingsRepo(db *sqlx.DB) SettingsRepo {
	return &sqliteSettingsRepo{db: db}
}

func (r *sqliteSettingsRepo) Get(ctx context.Context, key string) (string, error) {
	var value string
	err := r.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("settings get %q: %w", key, err)
	}
	return value, nil
}

func (r *sqliteSettingsRepo) Set(ctx context.Context, key, value string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		key, value, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("settings set %q: %w", key, err)
	}
	return nil
}

func (r *sqliteSettingsRepo) GetJSON(ctx context.Context, key string, out any) error {
	raw, err := r.Get(ctx, key)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("settings decode %q: %w", key, err)
	}
	return nil
}

func (r *sqliteSettingsRepo) SetJSON(ctx context.Context, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("settings encode %q: %w", key, err)
	}
	return r.Set(ctx, key, string(b))
}
