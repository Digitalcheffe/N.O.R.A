package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// ComponentLinkRepo manages the component_links table.
type ComponentLinkRepo interface {
	// SetParent upserts the parent for a child entity. If the child already has
	// a parent the existing row is replaced.
	SetParent(ctx context.Context, parentType, parentID, childType, childID string) error
	// RemoveParent deletes the parent link for a child entity.
	RemoveParent(ctx context.Context, childType, childID string) error
	// GetParent returns the parent link for a child, or ErrNotFound.
	GetParent(ctx context.Context, childType, childID string) (*models.ComponentLink, error)
	// GetChildren returns all direct children of a parent entity.
	GetChildren(ctx context.Context, parentType, parentID string) ([]models.ComponentLink, error)
	// GetChildrenOfType returns children of a given child_type under a parent.
	GetChildrenOfType(ctx context.Context, parentType, parentID, childType string) ([]models.ComponentLink, error)
	// ListAll returns every row in component_links (used by topology endpoint).
	ListAll(ctx context.Context) ([]models.ComponentLink, error)
}

type sqliteComponentLinkRepo struct{ db *sqlx.DB }

// NewComponentLinkRepo returns a ComponentLinkRepo backed by SQLite.
func NewComponentLinkRepo(db *sqlx.DB) ComponentLinkRepo {
	return &sqliteComponentLinkRepo{db: db}
}

func (r *sqliteComponentLinkRepo) SetParent(ctx context.Context, parentType, parentID, childType, childID string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO component_links (parent_type, parent_id, child_type, child_id, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(child_type, child_id) DO UPDATE SET
		    parent_type = excluded.parent_type,
		    parent_id   = excluded.parent_id`,
		parentType, parentID, childType, childID,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("set parent %s/%s → %s/%s: %w", parentType, parentID, childType, childID, err)
	}
	return nil
}

func (r *sqliteComponentLinkRepo) RemoveParent(ctx context.Context, childType, childID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM component_links WHERE child_type = ? AND child_id = ?`,
		childType, childID)
	if err != nil {
		return fmt.Errorf("remove parent for %s/%s: %w", childType, childID, err)
	}
	return nil
}

func (r *sqliteComponentLinkRepo) GetParent(ctx context.Context, childType, childID string) (*models.ComponentLink, error) {
	var link models.ComponentLink
	err := r.db.GetContext(ctx, &link, `
		SELECT parent_type, parent_id, child_type, child_id, created_at
		FROM component_links
		WHERE child_type = ? AND child_id = ?`,
		childType, childID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get parent for %s/%s: %w", childType, childID, err)
	}
	return &link, nil
}

func (r *sqliteComponentLinkRepo) GetChildren(ctx context.Context, parentType, parentID string) ([]models.ComponentLink, error) {
	var rows []models.ComponentLink
	err := r.db.SelectContext(ctx, &rows, `
		SELECT parent_type, parent_id, child_type, child_id, created_at
		FROM component_links
		WHERE parent_type = ? AND parent_id = ?
		ORDER BY child_type ASC, child_id ASC`,
		parentType, parentID)
	if err != nil {
		return nil, fmt.Errorf("get children for %s/%s: %w", parentType, parentID, err)
	}
	if rows == nil {
		rows = []models.ComponentLink{}
	}
	return rows, nil
}

func (r *sqliteComponentLinkRepo) GetChildrenOfType(ctx context.Context, parentType, parentID, childType string) ([]models.ComponentLink, error) {
	var rows []models.ComponentLink
	err := r.db.SelectContext(ctx, &rows, `
		SELECT parent_type, parent_id, child_type, child_id, created_at
		FROM component_links
		WHERE parent_type = ? AND parent_id = ? AND child_type = ?
		ORDER BY child_id ASC`,
		parentType, parentID, childType)
	if err != nil {
		return nil, fmt.Errorf("get children of type %s for %s/%s: %w", childType, parentType, parentID, err)
	}
	if rows == nil {
		rows = []models.ComponentLink{}
	}
	return rows, nil
}

func (r *sqliteComponentLinkRepo) ListAll(ctx context.Context) ([]models.ComponentLink, error) {
	var rows []models.ComponentLink
	err := r.db.SelectContext(ctx, &rows, `
		SELECT parent_type, parent_id, child_type, child_id, created_at
		FROM component_links
		ORDER BY parent_type ASC, parent_id ASC, child_type ASC, child_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list all component_links: %w", err)
	}
	if rows == nil {
		rows = []models.ComponentLink{}
	}
	return rows, nil
}
