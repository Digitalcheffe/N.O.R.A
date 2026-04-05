package repo

import (
	"context"
	"fmt"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// DigestRegistryRepo defines operations for the digest registry.
type DigestRegistryRepo interface {
	Upsert(ctx context.Context, entry models.DigestRegistryEntry) error
	SetActive(ctx context.Context, profileID string, name string, active bool) error
	List(ctx context.Context) ([]models.DigestRegistryEntry, error)
	ListByProfile(ctx context.Context, profileID string) ([]models.DigestRegistryEntry, error)
	Delete(ctx context.Context, id string) error
	SetActiveByID(ctx context.Context, id string, active bool) error
}

type sqliteDigestRegistryRepo struct {
	db *sqlx.DB
}

// NewDigestRegistryRepo returns a DigestRegistryRepo backed by the given SQLite database.
func NewDigestRegistryRepo(db *sqlx.DB) DigestRegistryRepo {
	return &sqliteDigestRegistryRepo{db: db}
}

// Upsert inserts or replaces a digest registry entry on the UNIQUE(profile_id, name) constraint.
func (r *sqliteDigestRegistryRepo) Upsert(ctx context.Context, entry models.DigestRegistryEntry) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO digest_registry (id, profile_id, source, entry_type, name, label, config, profile_source, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(profile_id, name) DO UPDATE SET
			label          = excluded.label,
			config         = excluded.config,
			profile_source = excluded.profile_source,
			active         = excluded.active,
			updated_at     = excluded.updated_at`,
		entry.ID, entry.ProfileID, entry.Source, entry.EntryType,
		entry.Name, entry.Label, string(entry.Config), entry.ProfileSource, entry.Active,
		entry.CreatedAt.UTC(), entry.UpdatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("upsert digest registry entry: %w", err)
	}
	return nil
}

// SetActive updates the active flag for an entry identified by profile_id+name.
func (r *sqliteDigestRegistryRepo) SetActive(ctx context.Context, profileID string, name string, active bool) error {
	activeInt := 0
	if active {
		activeInt = 1
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE digest_registry SET active = ?, updated_at = datetime('now')
		WHERE profile_id = ? AND name = ?`,
		activeInt, profileID, name,
	)
	if err != nil {
		return fmt.Errorf("set active digest registry entry: %w", err)
	}
	return nil
}

// List returns all digest registry entries ordered by profile_id, entry_type, name.
func (r *sqliteDigestRegistryRepo) List(ctx context.Context) ([]models.DigestRegistryEntry, error) {
	var entries []models.DigestRegistryEntry
	err := r.db.SelectContext(ctx, &entries, `
		SELECT id, profile_id, source, entry_type, name, label, config, profile_source, active, created_at, updated_at
		FROM digest_registry
		ORDER BY profile_id, entry_type, name`)
	if err != nil {
		return nil, fmt.Errorf("list digest registry entries: %w", err)
	}
	return entries, nil
}

// ListByProfile returns all entries for a given profile_id.
func (r *sqliteDigestRegistryRepo) ListByProfile(ctx context.Context, profileID string) ([]models.DigestRegistryEntry, error) {
	var entries []models.DigestRegistryEntry
	err := r.db.SelectContext(ctx, &entries, `
		SELECT id, profile_id, source, entry_type, name, label, config, profile_source, active, created_at, updated_at
		FROM digest_registry
		WHERE profile_id = ?
		ORDER BY entry_type, name`, profileID)
	if err != nil {
		return nil, fmt.Errorf("list digest registry entries by profile: %w", err)
	}
	return entries, nil
}

// Delete removes an inactive entry. Returns ErrConflict if the entry is still active.
func (r *sqliteDigestRegistryRepo) Delete(ctx context.Context, id string) error {
	// Check active status first.
	var active int
	err := r.db.QueryRowContext(ctx, `SELECT active FROM digest_registry WHERE id = ?`, id).Scan(&active)
	if err != nil {
		return fmt.Errorf("check digest registry entry: %w", err)
	}
	if active == 1 {
		return ErrConflict
	}
	res, err := r.db.ExecContext(ctx, `DELETE FROM digest_registry WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete digest registry entry: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetActiveByID updates the active flag for an entry identified by id.
func (r *sqliteDigestRegistryRepo) SetActiveByID(ctx context.Context, id string, active bool) error {
	activeInt := 0
	if active {
		activeInt = 1
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE digest_registry SET active = ?, updated_at = datetime('now') WHERE id = ?`,
		activeInt, id,
	)
	if err != nil {
		return fmt.Errorf("set active by id digest registry: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
