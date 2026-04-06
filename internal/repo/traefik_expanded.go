package repo

import (
	"context"
	"fmt"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/jmoiron/sqlx"
)

// ── TraefikOverviewRepo ───────────────────────────────────────────────────────

// TraefikOverviewRepo manages the traefik_overview table.
type TraefikOverviewRepo interface {
	// Upsert replaces the overview row for a Traefik component.
	Upsert(ctx context.Context, ov *models.TraefikOverview) error
	// Get returns the current overview for a component, or ErrNotFound.
	Get(ctx context.Context, componentID string) (*models.TraefikOverview, error)
}

type sqliteTraefikOverviewRepo struct{ db *sqlx.DB }

// NewTraefikOverviewRepo returns a TraefikOverviewRepo backed by SQLite.
func NewTraefikOverviewRepo(db *sqlx.DB) TraefikOverviewRepo {
	return &sqliteTraefikOverviewRepo{db: db}
}

func (r *sqliteTraefikOverviewRepo) Upsert(ctx context.Context, ov *models.TraefikOverview) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO traefik_overview
		  (component_id, version, routers_total, routers_errors, routers_warnings,
		   services_total, services_errors, middlewares_total, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(component_id) DO UPDATE SET
		  version           = excluded.version,
		  routers_total     = excluded.routers_total,
		  routers_errors    = excluded.routers_errors,
		  routers_warnings  = excluded.routers_warnings,
		  services_total    = excluded.services_total,
		  services_errors   = excluded.services_errors,
		  middlewares_total = excluded.middlewares_total,
		  updated_at        = excluded.updated_at`,
		ov.ComponentID, ov.Version,
		ov.RoutersTotal, ov.RoutersErrors, ov.RoutersWarnings,
		ov.ServicesTotal, ov.ServicesErrors,
		ov.MiddlewaresTotal, ov.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert traefik overview for %s: %w", ov.ComponentID, err)
	}
	return nil
}

func (r *sqliteTraefikOverviewRepo) Get(ctx context.Context, componentID string) (*models.TraefikOverview, error) {
	var ov models.TraefikOverview
	err := r.db.GetContext(ctx, &ov, `
		SELECT component_id, version,
		       routers_total, routers_errors, routers_warnings,
		       services_total, services_errors,
		       middlewares_total, updated_at
		FROM traefik_overview
		WHERE component_id = ?`, componentID)
	if err != nil {
		return nil, fmt.Errorf("get traefik overview for %s: %w", componentID, ErrNotFound)
	}
	return &ov, nil
}

// ── TraefikServiceRepo ────────────────────────────────────────────────────────

// TraefikServiceRepo manages the traefik_services table.
type TraefikServiceRepo interface {
	// Upsert replaces a single service row.
	Upsert(ctx context.Context, svc *models.TraefikService) error
	// ListByComponent returns all services for a component.
	// If statusFilter is non-empty ("down"), only rows where servers_down > 0 are returned.
	ListByComponent(ctx context.Context, componentID string, statusFilter string) ([]*models.TraefikService, error)
	// DeleteAbsent removes services for componentID whose service_name is not in present.
	// Used to clean up services that have disappeared from Traefik.
	DeleteAbsent(ctx context.Context, componentID string, present []string) error
}

type sqliteTraefikServiceRepo struct{ db *sqlx.DB }

// NewTraefikServiceRepo returns a TraefikServiceRepo backed by SQLite.
func NewTraefikServiceRepo(db *sqlx.DB) TraefikServiceRepo {
	return &sqliteTraefikServiceRepo{db: db}
}

func (r *sqliteTraefikServiceRepo) Upsert(ctx context.Context, svc *models.TraefikService) error {
	if svc.FirstSeenAt == nil {
		now := svc.LastSeen
		svc.FirstSeenAt = &now
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO traefik_services
		  (id, component_id, service_name, service_type, status,
		   server_count, servers_up, servers_down, server_status_json, last_seen, first_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  service_type       = excluded.service_type,
		  status             = excluded.status,
		  server_count       = excluded.server_count,
		  servers_up         = excluded.servers_up,
		  servers_down       = excluded.servers_down,
		  server_status_json = excluded.server_status_json,
		  last_seen          = excluded.last_seen`,
		svc.ID, svc.ComponentID, svc.ServiceName, svc.ServiceType, svc.Status,
		svc.ServerCount, svc.ServersUp, svc.ServersDown,
		svc.ServerStatusJSON, svc.LastSeen, svc.FirstSeenAt)
	if err != nil {
		return fmt.Errorf("upsert traefik service %s: %w", svc.ID, err)
	}
	return nil
}

func (r *sqliteTraefikServiceRepo) ListByComponent(ctx context.Context, componentID string, statusFilter string) ([]*models.TraefikService, error) {
	var rows []*models.TraefikService
	var err error
	if statusFilter == "down" {
		err = r.db.SelectContext(ctx, &rows, `
			SELECT id, component_id, service_name, service_type, status,
			       server_count, servers_up, servers_down,
			       COALESCE(server_status_json,'{}') AS server_status_json, last_seen, first_seen_at
			FROM traefik_services
			WHERE component_id = ? AND servers_down > 0
			ORDER BY service_name ASC`, componentID)
	} else {
		err = r.db.SelectContext(ctx, &rows, `
			SELECT id, component_id, service_name, service_type, status,
			       server_count, servers_up, servers_down,
			       COALESCE(server_status_json,'{}') AS server_status_json, last_seen, first_seen_at
			FROM traefik_services
			WHERE component_id = ?
			ORDER BY service_name ASC`, componentID)
	}
	if err != nil {
		return nil, fmt.Errorf("list traefik services for %s: %w", componentID, err)
	}
	if rows == nil {
		rows = []*models.TraefikService{}
	}
	return rows, nil
}

func (r *sqliteTraefikServiceRepo) DeleteAbsent(ctx context.Context, componentID string, present []string) error {
	if len(present) == 0 {
		// All services gone — delete everything for this component.
		_, err := r.db.ExecContext(ctx,
			`DELETE FROM traefik_services WHERE component_id = ?`, componentID)
		return err
	}

	// Build a parameterised NOT IN clause.
	query := `DELETE FROM traefik_services WHERE component_id = ? AND service_name NOT IN (`
	args := []interface{}{componentID}
	for i, name := range present {
		if i > 0 {
			query += ","
		}
		query += "?"
		args = append(args, name)
	}
	query += ")"
	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("delete absent traefik services for %s: %w", componentID, err)
	}
	return nil
}

