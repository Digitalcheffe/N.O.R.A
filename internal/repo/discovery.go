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
	// ListStoppedContainers returns every row whose status is not "running".
	// Used by the cleanup-jobs preview so the UI can show which ghosts will
	// be pruned before the job fires.
	ListStoppedContainers(ctx context.Context) ([]*models.DiscoveredContainer, error)
	// DeleteAllStoppedContainers hard-deletes every row whose status is not
	// "running". Returns the number of rows removed.
	DeleteAllStoppedContainers(ctx context.Context) (int64, error)
	// UpdateContainerLocalDigest stores the locally running image manifest digest.
	// Called by Portainer/Docker Engine enrichment after inspecting the container.
	// Does not touch registry_digest or image_update_available.
	UpdateContainerLocalDigest(ctx context.Context, id string, imageDigest string) error
	// UpdateContainerImageCheck writes the latest image and registry digests and
	// sets image_update_available.  Called by the ImageUpdatePoller after each poll.
	UpdateContainerImageCheck(ctx context.Context, id string, imageDigest string, registryDigest string, updateAvailable bool) error
	// UpdateContainerRestartPolicy persists the container restart policy.
	// Called by the ImageUpdatePoller which already performs a ContainerInspect.
	UpdateContainerRestartPolicy(ctx context.Context, id string, policy string) error
	// UpdateContainerEnvVars persists the JSON-encoded environment variable list.
	// Called by the Portainer enrichment worker after InspectContainer.
	// Does not touch any other fields.
	UpdateContainerEnvVars(ctx context.Context, id string, envVars string) error
}

// DiscoveredRouteRepo manages the discovered_routes table.
type DiscoveredRouteRepo interface {
	UpsertDiscoveredRoute(ctx context.Context, r *models.DiscoveredRoute) error
	ListDiscoveredRoutes(ctx context.Context, infrastructureID string) ([]*models.DiscoveredRoute, error)
	// ListDiscoveredRoutesByStatus returns routes for a component filtered by router_status.
	// If statusFilter is empty, all routes are returned.
	ListDiscoveredRoutesByStatus(ctx context.Context, infrastructureID string, statusFilter string) ([]*models.DiscoveredRoute, error)
	ListAllDiscoveredRoutes(ctx context.Context) ([]*models.DiscoveredRoute, error)
	GetDiscoveredRoute(ctx context.Context, id string) (*models.DiscoveredRoute, error)
	// ListByAppID returns all discovered routes linked to appID, ordered by router_name.
	ListByAppID(ctx context.Context, appID string) ([]*models.DiscoveredRoute, error)
	SetDiscoveredRouteApp(ctx context.Context, id string, appID string) error
	ClearDiscoveredRouteApp(ctx context.Context, id string) error
	// SyncRouteAppLink ensures a traefik_route → app entry exists in component_links
	// for the route identified by (infrastructureID, routerName). Called by the
	// Traefik discovery scanner after auto-resolving an app via container cross-ref.
	SyncRouteAppLink(ctx context.Context, infrastructureID, routerName, appID string)
	// ListServicesForComponent returns a deduplicated service summary for a Traefik
	// component, derived from discovered_routes grouped by service_name.
	// If statusFilter is "down", only services with servers_down > 0 are returned.
	ListServicesForComponent(ctx context.Context, infrastructureID string, statusFilter string) ([]*models.DiscoveredServiceSummary, error)
}

// ── DiscoveredContainerRepo implementation ────────────────────────────────────

// containerSelectCols is the shared column list for discovered_containers SELECT queries.
const containerSelectCols = `id, infra_component_id, source_type, container_id, container_name, image, status,
	app_id, profile_suggestion, suggestion_confidence, last_seen_at, created_at,
	image_digest, registry_digest, COALESCE(image_update_available,0) AS image_update_available, image_last_checked_at,
	ports, labels, volumes, networks, restart_policy, docker_created_at, env_vars`

type sqliteDiscoveredContainerRepo struct{ db *sqlx.DB }

// NewDiscoveredContainerRepo returns a DiscoveredContainerRepo backed by SQLite.
func NewDiscoveredContainerRepo(db *sqlx.DB) DiscoveredContainerRepo {
	return &sqliteDiscoveredContainerRepo{db: db}
}

func (r *sqliteDiscoveredContainerRepo) UpsertDiscoveredContainer(ctx context.Context, c *models.DiscoveredContainer) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	// Conflict key is (infra_component_id, container_name): the name is stable
	// across container rebuilds; container_id (the Docker hash) changes and is
	// refreshed on every upsert.  RETURNING id gives the caller the stable NORA
	// UUID whether this was an INSERT or an UPDATE.
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO discovered_containers
		  (id, infra_component_id, source_type, container_id, container_name, image, status,
		   app_id, profile_suggestion, suggestion_confidence, last_seen_at, created_at,
		   ports, labels, volumes, networks, restart_policy, docker_created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(infra_component_id, container_name) DO UPDATE SET
		  source_type           = excluded.source_type,
		  container_id          = excluded.container_id,
		  image                 = excluded.image,
		  status                = excluded.status,
		  profile_suggestion    = excluded.profile_suggestion,
		  suggestion_confidence = excluded.suggestion_confidence,
		  last_seen_at          = excluded.last_seen_at,
		  ports                 = COALESCE(excluded.ports, ports),
		  labels                = COALESCE(excluded.labels, labels),
		  volumes               = COALESCE(excluded.volumes, volumes),
		  networks              = COALESCE(excluded.networks, networks),
		  docker_created_at     = COALESCE(excluded.docker_created_at, docker_created_at)
		RETURNING id`,
		c.ID, c.InfraComponentID, c.SourceType, c.ContainerID, c.ContainerName, c.Image, c.Status,
		c.AppID, c.ProfileSuggestion, c.SuggestionConfidence, c.LastSeenAt, c.CreatedAt,
		c.Ports, c.Labels, c.Volumes, c.Networks, c.RestartPolicy, c.DockerCreatedAt,
	).Scan(&c.ID)
	if err != nil {
		return fmt.Errorf("upsert discovered container %s: %w", c.ContainerName, err)
	}
	// Keep component_links in sync: container → parent infra_component.
	var parentType string
	if err2 := r.db.QueryRowContext(ctx,
		`SELECT type FROM infrastructure_components WHERE id = ?`, c.InfraComponentID,
	).Scan(&parentType); err2 == nil {
		r.db.ExecContext(ctx, `
			INSERT INTO component_links (parent_type, parent_id, child_type, child_id, created_at)
			VALUES (?, ?, 'container', ?, datetime('now'))
			ON CONFLICT(child_type, child_id) DO UPDATE SET parent_type=excluded.parent_type, parent_id=excluded.parent_id`,
			parentType, c.InfraComponentID, c.ID)
	}
	return nil
}

func (r *sqliteDiscoveredContainerRepo) ListDiscoveredContainers(ctx context.Context, infraComponentID string) ([]*models.DiscoveredContainer, error) {
	var rows []*models.DiscoveredContainer
	err := r.db.SelectContext(ctx, &rows, `
		SELECT `+containerSelectCols+`
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
		SELECT `+containerSelectCols+`
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
		SELECT `+containerSelectCols+`
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
	// Keep component_links in sync: app → container.
	r.db.ExecContext(ctx, `
		INSERT INTO component_links (parent_type, parent_id, child_type, child_id, created_at)
		VALUES ('container', ?, 'app', ?, datetime('now'))
		ON CONFLICT(child_type, child_id) DO UPDATE SET parent_type='container', parent_id=excluded.parent_id`,
		id, appID)
	return nil
}

func (r *sqliteDiscoveredContainerRepo) ClearDiscoveredContainerApp(ctx context.Context, id string) error {
	// Resolve app_id before clearing so we can remove it from component_links.
	var appID string
	r.db.QueryRowContext(ctx, `SELECT app_id FROM discovered_containers WHERE id = ?`, id).Scan(&appID)

	res, err := r.db.ExecContext(ctx,
		`UPDATE discovered_containers SET app_id = NULL WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("clear app on discovered container %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("discovered container %s: %w", id, ErrNotFound)
	}
	// Remove the app → container link from component_links.
	if appID != "" {
		r.db.ExecContext(ctx, `DELETE FROM component_links WHERE child_type = 'app' AND child_id = ?`, appID)
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
		SELECT `+containerSelectCols+`
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

func (r *sqliteDiscoveredContainerRepo) UpdateContainerLocalDigest(ctx context.Context, id string, imageDigest string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE discovered_containers SET image_digest = ? WHERE id = ?`, imageDigest, id)
	if err != nil {
		return fmt.Errorf("update local digest on container %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("discovered container %s: %w", id, ErrNotFound)
	}
	return nil
}

func (r *sqliteDiscoveredContainerRepo) UpdateContainerImageCheck(ctx context.Context, id string, imageDigest string, registryDigest string, updateAvailable bool) error {
	updateAvailableInt := 0
	if updateAvailable {
		updateAvailableInt = 1
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE discovered_containers
		SET image_digest = ?, registry_digest = ?,
		    image_update_available = ?, image_last_checked_at = ?
		WHERE id = ?`,
		imageDigest, registryDigest, updateAvailableInt, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("update image check on container %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("discovered container %s: %w", id, ErrNotFound)
	}
	return nil
}

func (r *sqliteDiscoveredContainerRepo) UpdateContainerRestartPolicy(ctx context.Context, id string, policy string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE discovered_containers SET restart_policy = ? WHERE id = ?`, policy, id)
	if err != nil {
		return fmt.Errorf("update restart policy on container %s: %w", id, err)
	}
	return nil
}

func (r *sqliteDiscoveredContainerRepo) UpdateContainerEnvVars(ctx context.Context, id string, envVars string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE discovered_containers SET env_vars = ? WHERE id = ?`, envVars, id)
	if err != nil {
		return fmt.Errorf("update env vars on container %s: %w", id, err)
	}
	return nil
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

func (r *sqliteDiscoveredContainerRepo) ListStoppedContainers(ctx context.Context) ([]*models.DiscoveredContainer, error) {
	var rows []*models.DiscoveredContainer
	err := r.db.SelectContext(ctx, &rows, `
		SELECT id, infra_component_id, source_type, container_id, container_name,
		       image, status, app_id, profile_suggestion, suggestion_confidence,
		       last_seen_at, created_at, labels, ports, networks, volumes,
		       env_vars, image_digest, registry_digest, image_last_checked_at,
		       image_update_available, restart_policy
		FROM discovered_containers
		WHERE status != 'running'
		ORDER BY last_seen_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list stopped discovered containers: %w", err)
	}
	return rows, nil
}

func (r *sqliteDiscoveredContainerRepo) DeleteAllStoppedContainers(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM discovered_containers WHERE status != 'running'`)
	if err != nil {
		return 0, fmt.Errorf("delete all stopped discovered containers: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ── DiscoveredRouteRepo implementation ───────────────────────────────────────

type sqliteDiscoveredRouteRepo struct{ db *sqlx.DB }

// NewDiscoveredRouteRepo returns a DiscoveredRouteRepo backed by SQLite.
func NewDiscoveredRouteRepo(db *sqlx.DB) DiscoveredRouteRepo {
	return &sqliteDiscoveredRouteRepo{db: db}
}

// routeSelectCols is the shared column list for discovered_routes SELECT queries.
const routeSelectCols = `id, infrastructure_id, router_name, rule,
	domain, container_id, app_id, ssl_expiry, ssl_issuer,
	last_seen_at, created_at,
	COALESCE(router_status,'enabled') AS router_status,
	provider, entry_points, COALESCE(has_tls_resolver,0) AS has_tls_resolver,
	cert_resolver_name, service_name,
	service_status, service_type,
	COALESCE(servers_total,0) AS servers_total,
	COALESCE(servers_up,0)    AS servers_up,
	COALESCE(servers_down,0)  AS servers_down,
	servers_json`

func (r *sqliteDiscoveredRouteRepo) UpsertDiscoveredRoute(ctx context.Context, ro *models.DiscoveredRoute) error {
	if ro.ID == "" {
		ro.ID = uuid.New().String()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO discovered_routes
		  (id, infrastructure_id, router_name, rule, domain,
		   container_id, app_id, ssl_expiry, ssl_issuer, last_seen_at, created_at,
		   router_status, provider, entry_points, has_tls_resolver, cert_resolver_name,
		   service_name, service_status, service_type, servers_total, servers_up, servers_down,
		   servers_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(infrastructure_id, router_name) DO UPDATE SET
		  rule               = excluded.rule,
		  domain             = excluded.domain,
		  container_id       = excluded.container_id,
		  ssl_expiry         = excluded.ssl_expiry,
		  ssl_issuer         = excluded.ssl_issuer,
		  last_seen_at       = excluded.last_seen_at,
		  router_status      = excluded.router_status,
		  provider           = excluded.provider,
		  entry_points       = excluded.entry_points,
		  has_tls_resolver   = excluded.has_tls_resolver,
		  cert_resolver_name = excluded.cert_resolver_name,
		  service_name       = excluded.service_name,
		  service_status     = excluded.service_status,
		  service_type       = excluded.service_type,
		  servers_total      = excluded.servers_total,
		  servers_up         = excluded.servers_up,
		  servers_down       = excluded.servers_down,
		  servers_json       = excluded.servers_json`,
		ro.ID, ro.InfrastructureID, ro.RouterName, ro.Rule, ro.Domain,
		ro.ContainerID, ro.AppID, ro.SSLExpiry, ro.SSLIssuer, ro.LastSeenAt, ro.CreatedAt,
		ro.RouterStatus, ro.Provider, ro.EntryPoints, ro.HasTLSResolver,
		ro.CertResolverName, ro.ServiceName, ro.ServiceStatus, ro.ServiceType,
		ro.ServersTotal, ro.ServersUp, ro.ServersDown, ro.ServersJSON)
	if err != nil {
		return fmt.Errorf("upsert discovered route %s: %w", ro.RouterName, err)
	}
	// Keep component_links in sync: traefik infra_component → traefik_route.
	// Use a subquery to get the actual row ID (which may differ from ro.ID on conflict).
	r.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO component_links (parent_type, parent_id, child_type, child_id, created_at)
		SELECT 'traefik', ?, 'traefik_route', id, datetime('now')
		FROM discovered_routes
		WHERE infrastructure_id = ? AND router_name = ?`,
		ro.InfrastructureID, ro.InfrastructureID, ro.RouterName)
	return nil
}

func (r *sqliteDiscoveredRouteRepo) SyncRouteAppLink(ctx context.Context, infrastructureID, routerName, appID string) {
	// INSERT OR IGNORE so we don't override an existing container → app parent.
	r.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO component_links (parent_type, parent_id, child_type, child_id, created_at)
		SELECT 'traefik_route', id, 'app', ?, datetime('now')
		FROM discovered_routes
		WHERE infrastructure_id = ? AND router_name = ?`,
		appID, infrastructureID, routerName)
}

func (r *sqliteDiscoveredRouteRepo) ListDiscoveredRoutes(ctx context.Context, infrastructureID string) ([]*models.DiscoveredRoute, error) {
	var rows []*models.DiscoveredRoute
	err := r.db.SelectContext(ctx, &rows, `
		SELECT `+routeSelectCols+`
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

func (r *sqliteDiscoveredRouteRepo) ListDiscoveredRoutesByStatus(ctx context.Context, infrastructureID string, statusFilter string) ([]*models.DiscoveredRoute, error) {
	var rows []*models.DiscoveredRoute
	var err error
	if statusFilter != "" {
		err = r.db.SelectContext(ctx, &rows, `
			SELECT `+routeSelectCols+`
			FROM discovered_routes
			WHERE infrastructure_id = ? AND COALESCE(router_status,'enabled') = ?
			ORDER BY router_name ASC`, infrastructureID, statusFilter)
	} else {
		err = r.db.SelectContext(ctx, &rows, `
			SELECT `+routeSelectCols+`
			FROM discovered_routes
			WHERE infrastructure_id = ?
			ORDER BY router_name ASC`, infrastructureID)
	}
	if err != nil {
		return nil, fmt.Errorf("list discovered routes by status for infra %s: %w", infrastructureID, err)
	}
	if rows == nil {
		rows = []*models.DiscoveredRoute{}
	}
	return rows, nil
}

func (r *sqliteDiscoveredRouteRepo) ListAllDiscoveredRoutes(ctx context.Context) ([]*models.DiscoveredRoute, error) {
	var rows []*models.DiscoveredRoute
	err := r.db.SelectContext(ctx, &rows, `
		SELECT `+routeSelectCols+`
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
		SELECT `+routeSelectCols+`
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

func (r *sqliteDiscoveredRouteRepo) ListByAppID(ctx context.Context, appID string) ([]*models.DiscoveredRoute, error) {
	var routes []*models.DiscoveredRoute
	// Match directly by app_id OR via container_id → discovered_containers.app_id
	// (the latter handles routes discovered before app_id propagation was added).
	err := r.db.SelectContext(ctx, &routes, `
		SELECT `+routeSelectCols+`
		FROM discovered_routes
		WHERE app_id = ?
		   OR container_id IN (
		        SELECT id FROM discovered_containers WHERE app_id = ?
		   )
		ORDER BY router_name`, appID, appID)
	if err != nil {
		return nil, fmt.Errorf("list discovered routes for app %s: %w", appID, err)
	}
	return routes, nil
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
	// Keep component_links in sync: traefik_route → app.
	// INSERT OR IGNORE so we don't override an existing container → app parent link.
	r.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO component_links (parent_type, parent_id, child_type, child_id, created_at)
		VALUES ('traefik_route', ?, 'app', ?, datetime('now'))`,
		id, appID)
	return nil
}

func (r *sqliteDiscoveredRouteRepo) ClearDiscoveredRouteApp(ctx context.Context, id string) error {
	// Resolve app_id before clearing so we can remove the specific link from component_links.
	var appID string
	r.db.QueryRowContext(ctx, `SELECT COALESCE(app_id,'') FROM discovered_routes WHERE id = ?`, id).Scan(&appID)

	res, err := r.db.ExecContext(ctx,
		`UPDATE discovered_routes SET app_id = NULL WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("clear app on discovered route %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("discovered route %s: %w", id, ErrNotFound)
	}
	// Remove the traefik_route → app link from component_links.
	// Only remove if this specific route was the parent (don't touch a container → app link).
	if appID != "" {
		r.db.ExecContext(ctx, `
			DELETE FROM component_links
			WHERE child_type = 'app' AND child_id = ? AND parent_type = 'traefik_route' AND parent_id = ?`,
			appID, id)
	}
	return nil
}

func (r *sqliteDiscoveredRouteRepo) ListServicesForComponent(ctx context.Context, infrastructureID string, statusFilter string) ([]*models.DiscoveredServiceSummary, error) {
	baseQuery := `
		SELECT
		    infrastructure_id || ':' || service_name        AS id,
		    infrastructure_id                               AS component_id,
		    service_name,
		    COALESCE(MAX(service_type), 'loadbalancer')    AS service_type,
		    COALESCE(MAX(service_status), 'enabled')       AS status,
		    COALESCE(MAX(servers_total), 0)                AS server_count,
		    COALESCE(MAX(servers_up), 0)                   AS servers_up,
		    COALESCE(MAX(servers_down), 0)                 AS servers_down,
		    MAX(last_seen_at)                              AS last_seen
		FROM discovered_routes
		WHERE infrastructure_id = ? AND service_name IS NOT NULL
		GROUP BY service_name`

	var rows []*models.DiscoveredServiceSummary
	var err error
	if statusFilter == "down" {
		err = r.db.SelectContext(ctx, &rows, baseQuery+`
		HAVING COALESCE(MAX(servers_down), 0) > 0
		ORDER BY service_name ASC`, infrastructureID)
	} else {
		err = r.db.SelectContext(ctx, &rows, baseQuery+`
		ORDER BY service_name ASC`, infrastructureID)
	}
	if err != nil {
		return nil, fmt.Errorf("list services for infra %s: %w", infrastructureID, err)
	}
	for _, s := range rows {
		s.ServerStatusJSON = "{}"
	}
	if rows == nil {
		rows = []*models.DiscoveredServiceSummary{}
	}
	return rows, nil
}
