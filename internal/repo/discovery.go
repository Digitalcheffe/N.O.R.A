package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// ── Interfaces ────────────────────────────────────────────────────────────────

// DiscoveredContainerRepo manages the discovered_containers table.
type DiscoveredContainerRepo interface {
	UpsertDiscoveredContainer(ctx context.Context, c *models.DiscoveredContainer) error
	ListDiscoveredContainers(ctx context.Context, infraComponentID string) ([]*models.DiscoveredContainer, error)
	ListAllDiscoveredContainers(ctx context.Context) ([]*models.DiscoveredContainer, error)
	GetDiscoveredContainer(ctx context.Context, id string) (*models.DiscoveredContainer, error)
	// FindByName returns the first discovered container matching engineID+name, or ErrNotFound.
	FindByName(ctx context.Context, infraComponentID string, name string) (*models.DiscoveredContainer, error)
	SetDiscoveredContainerApp(ctx context.Context, id string, appID string) error
	ClearDiscoveredContainerApp(ctx context.Context, id string) error
	UpdateDiscoveredContainerStatus(ctx context.Context, id string, status string, lastSeenAt time.Time) error
	// MarkStoppedIfNotRunning sets status="stopped" for all containers belonging to
	// infraComponentID whose container_id is NOT in runningIDs.  Called after each
	// reconcile scan so containers removed from Docker don't stay as "running".
	MarkStoppedIfNotRunning(ctx context.Context, infraComponentID string, runningIDs []string) error
	// DeleteDiscoveredContainer hard-deletes a discovered container record by UUID.
	DeleteDiscoveredContainer(ctx context.Context, id string) error
}

// DiscoveredRouteRepo manages the discovered_routes table.
type DiscoveredRouteRepo interface {
	UpsertDiscoveredRoute(ctx context.Context, r *models.DiscoveredRoute) error
	ListDiscoveredRoutes(ctx context.Context, infrastructureID string) ([]*models.DiscoveredRoute, error)
	ListAllDiscoveredRoutes(ctx context.Context) ([]*models.DiscoveredRoute, error)
	GetDiscoveredRoute(ctx context.Context, id string) (*models.DiscoveredRoute, error)
	SetDiscoveredRouteApp(ctx context.Context, id string, appID string) error
	ClearDiscoveredRouteApp(ctx context.Context, id string) error
}

// ── DiscoveredContainerRepo implementation ────────────────────────────────────

type sqliteDiscoveredContainerRepo struct{ db *sqlx.DB }

// NewDiscoveredContainerRepo returns a DiscoveredContainerRepo backed by SQLite.
func NewDiscoveredContainerRepo(db *sqlx.DB) DiscoveredContainerRepo {
	return &sqliteDiscoveredContainerRepo{db: db}
}

func (r *sqliteDiscoveredContainerRepo) UpsertDiscoveredContainer(ctx context.Context, c *models.DiscoveredContainer) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO discovered_containers
		  (id, infra_component_id, container_id, container_name, image, status,
		   app_id, profile_suggestion, suggestion_confidence, last_seen_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(infra_component_id, container_id) DO UPDATE SET
		  container_name        = excluded.container_name,
		  image                 = excluded.image,
		  status                = excluded.status,
		  profile_suggestion    = excluded.profile_suggestion,
		  suggestion_confidence = excluded.suggestion_confidence,
		  last_seen_at          = excluded.last_seen_at`,
		c.ID, c.InfraComponentID, c.ContainerID, c.ContainerName, c.Image, c.Status,
		c.AppID, c.ProfileSuggestion, c.SuggestionConfidence, c.LastSeenAt, c.CreatedAt)
	if err != nil {
		return fmt.Errorf("upsert discovered container %s: %w", c.ContainerID, err)
	}
	return nil
}

func (r *sqliteDiscoveredContainerRepo) ListDiscoveredContainers(ctx context.Context, infraComponentID string) ([]*models.DiscoveredContainer, error) {
	var rows []*models.DiscoveredContainer
	err := r.db.SelectContext(ctx, &rows, `
		SELECT id, infra_component_id, container_id, container_name, image, status,
		       app_id, profile_suggestion, suggestion_confidence, last_seen_at, created_at
		FROM discovered_containers
		WHERE infra_component_id = ?
		ORDER BY container_name ASC`, infraComponentID)
	if err != nil {
		return nil, fmt.Errorf("list discovered containers for component %s: %w", infraComponentID, err)
	}
	if rows == nil {
		rows = []*models.DiscoveredContainer{}
	}
	return rows, nil
}

func (r *sqliteDiscoveredContainerRepo) ListAllDiscoveredContainers(ctx context.Context) ([]*models.DiscoveredContainer, error) {
	var rows []*models.DiscoveredContainer
	err := r.db.SelectContext(ctx, &rows, `
		SELECT id, infra_component_id, container_id, container_name, image, status,
		       app_id, profile_suggestion, suggestion_confidence, last_seen_at, created_at
		FROM discovered_containers
		ORDER BY infra_component_id ASC, container_name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list all discovered containers: %w", err)
	}
	if rows == nil {
		rows = []*models.DiscoveredContainer{}
	}
	return rows, nil
}

func (r *sqliteDiscoveredContainerRepo) GetDiscoveredContainer(ctx context.Context, id string) (*models.DiscoveredContainer, error) {
	var c models.DiscoveredContainer
	err := r.db.GetContext(ctx, &c, `
		SELECT id, infra_component_id, container_id, container_name, image, status,
		       app_id, profile_suggestion, suggestion_confidence, last_seen_at, created_at
		FROM discovered_containers
		WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("discovered container %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get discovered container %s: %w", id, err)
	}
	return &c, nil
}

func (r *sqliteDiscoveredContainerRepo) SetDiscoveredContainerApp(ctx context.Context, id string, appID string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE discovered_containers SET app_id = ? WHERE id = ?`, appID, id)
	if err != nil {
		return fmt.Errorf("set app on discovered container %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("discovered container %s: %w", id, ErrNotFound)
	}
	return nil
}

func (r *sqliteDiscoveredContainerRepo) ClearDiscoveredContainerApp(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE discovered_containers SET app_id = NULL WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("clear app on discovered container %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("discovered container %s: %w", id, ErrNotFound)
	}
	return nil
}

func (r *sqliteDiscoveredContainerRepo) UpdateDiscoveredContainerStatus(ctx context.Context, id string, status string, lastSeenAt time.Time) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE discovered_containers SET status = ?, last_seen_at = ? WHERE id = ?`,
		status, lastSeenAt, id)
	if err != nil {
		return fmt.Errorf("update status on discovered container %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("discovered container %s: %w", id, ErrNotFound)
	}
	return nil
}

func (r *sqliteDiscoveredContainerRepo) MarkStoppedIfNotRunning(ctx context.Context, infraComponentID string, runningIDs []string) error {
	if len(runningIDs) == 0 {
		// Nothing is running — mark everything for this engine as stopped.
		_, err := r.db.ExecContext(ctx, `
			UPDATE discovered_containers
			SET status = 'stopped', last_seen_at = ?
			WHERE infra_component_id = ? AND status = 'running'`,
			time.Now().UTC(), infraComponentID)
		return err
	}

	// Build a parameterised NOT IN clause.
	placeholders := make([]byte, 0, len(runningIDs)*2)
	args := make([]interface{}, 0, len(runningIDs)+2)
	args = append(args, time.Now().UTC(), infraComponentID)
	for i, id := range runningIDs {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args = append(args, id)
	}

	query := fmt.Sprintf(`
		UPDATE discovered_containers
		SET status = 'stopped', last_seen_at = ?
		WHERE infra_component_id = ? AND status = 'running'
		  AND container_id NOT IN (%s)`, string(placeholders))

	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

func (r *sqliteDiscoveredContainerRepo) FindByName(ctx context.Context, infraComponentID string, name string) (*models.DiscoveredContainer, error) {
	var c models.DiscoveredContainer
	err := r.db.GetContext(ctx, &c, `
		SELECT id, infra_component_id, container_id, container_name, image, status,
		       app_id, profile_suggestion, suggestion_confidence, last_seen_at, created_at
		FROM discovered_containers
		WHERE infra_component_id = ? AND container_name = ?
		LIMIT 1`, infraComponentID, name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("discovered container name %s: %w", name, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("find discovered container by name %s: %w", name, err)
	}
	return &c, nil
}

func (r *sqliteDiscoveredContainerRepo) DeleteDiscoveredContainer(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM discovered_containers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete discovered container %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("discovered container %s: %w", id, ErrNotFound)
	}
	return nil
}

// ── DiscoveredRouteRepo implementation ───────────────────────────────────────

type sqliteDiscoveredRouteRepo struct{ db *sqlx.DB }

// NewDiscoveredRouteRepo returns a DiscoveredRouteRepo backed by SQLite.
func NewDiscoveredRouteRepo(db *sqlx.DB) DiscoveredRouteRepo {
	return &sqliteDiscoveredRouteRepo{db: db}
}

func (r *sqliteDiscoveredRouteRepo) UpsertDiscoveredRoute(ctx context.Context, ro *models.DiscoveredRoute) error {
	if ro.ID == "" {
		ro.ID = uuid.New().String()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO discovered_routes
		  (id, infrastructure_id, router_name, rule, domain, backend_service,
		   container_id, app_id, ssl_expiry, ssl_issuer, last_seen_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(infrastructure_id, router_name) DO UPDATE SET
		  rule            = excluded.rule,
		  domain          = excluded.domain,
		  backend_service = excluded.backend_service,
		  container_id    = excluded.container_id,
		  ssl_expiry      = excluded.ssl_expiry,
		  ssl_issuer      = excluded.ssl_issuer,
		  last_seen_at    = excluded.last_seen_at`,
		ro.ID, ro.InfrastructureID, ro.RouterName, ro.Rule, ro.Domain, ro.BackendService,
		ro.ContainerID, ro.AppID, ro.SSLExpiry, ro.SSLIssuer, ro.LastSeenAt, ro.CreatedAt)
	if err != nil {
		return fmt.Errorf("upsert discovered route %s: %w", ro.RouterName, err)
	}
	return nil
}

func (r *sqliteDiscoveredRouteRepo) ListDiscoveredRoutes(ctx context.Context, infrastructureID string) ([]*models.DiscoveredRoute, error) {
	var rows []*models.DiscoveredRoute
	err := r.db.SelectContext(ctx, &rows, `
		SELECT id, infrastructure_id, router_name, rule, domain, backend_service,
		       container_id, app_id, ssl_expiry, ssl_issuer, last_seen_at, created_at
		FROM discovered_routes
		WHERE infrastructure_id = ?
		ORDER BY router_name ASC`, infrastructureID)
	if err != nil {
		return nil, fmt.Errorf("list discovered routes for infra %s: %w", infrastructureID, err)
	}
	if rows == nil {
		rows = []*models.DiscoveredRoute{}
	}
	return rows, nil
}

func (r *sqliteDiscoveredRouteRepo) ListAllDiscoveredRoutes(ctx context.Context) ([]*models.DiscoveredRoute, error) {
	var rows []*models.DiscoveredRoute
	err := r.db.SelectContext(ctx, &rows, `
		SELECT id, infrastructure_id, router_name, rule, domain, backend_service,
		       container_id, app_id, ssl_expiry, ssl_issuer, last_seen_at, created_at
		FROM discovered_routes
		ORDER BY infrastructure_id ASC, router_name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list all discovered routes: %w", err)
	}
	if rows == nil {
		rows = []*models.DiscoveredRoute{}
	}
	return rows, nil
}

func (r *sqliteDiscoveredRouteRepo) GetDiscoveredRoute(ctx context.Context, id string) (*models.DiscoveredRoute, error) {
	var ro models.DiscoveredRoute
	err := r.db.GetContext(ctx, &ro, `
		SELECT id, infrastructure_id, router_name, rule, domain, backend_service,
		       container_id, app_id, ssl_expiry, ssl_issuer, last_seen_at, created_at
		FROM discovered_routes
		WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("discovered route %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get discovered route %s: %w", id, err)
	}
	return &ro, nil
}

func (r *sqliteDiscoveredRouteRepo) SetDiscoveredRouteApp(ctx context.Context, id string, appID string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE discovered_routes SET app_id = ? WHERE id = ?`, appID, id)
	if err != nil {
		return fmt.Errorf("set app on discovered route %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("discovered route %s: %w", id, ErrNotFound)
	}
	return nil
}

func (r *sqliteDiscoveredRouteRepo) ClearDiscoveredRouteApp(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE discovered_routes SET app_id = NULL WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("clear app on discovered route %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("discovered route %s: %w", id, ErrNotFound)
	}
	return nil
}
