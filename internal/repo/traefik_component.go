package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// TraefikComponentRepo manages traefik_component_certs and traefik_routes.
type TraefikComponentRepo interface {
	// UpsertCerts replaces the cert cache for a traefik component.
	UpsertCerts(ctx context.Context, componentID string, certs []*models.TraefikComponentCert) error
	// ListCerts returns all cached certs for a traefik component.
	ListCerts(ctx context.Context, componentID string) ([]*models.TraefikComponentCert, error)

	// UpsertRoutes replaces the route cache for a traefik component.
	UpsertRoutes(ctx context.Context, componentID string, routes []models.TraefikRoute) error
	// ListRoutes returns all cached routes for a traefik component.
	ListRoutes(ctx context.Context, componentID string) ([]models.TraefikRoute, error)
}

type sqliteTraefikComponentRepo struct{ db *sqlx.DB }

// NewTraefikComponentRepo returns a TraefikComponentRepo backed by SQLite.
func NewTraefikComponentRepo(db *sqlx.DB) TraefikComponentRepo {
	return &sqliteTraefikComponentRepo{db: db}
}

// ── Certs ─────────────────────────────────────────────────────────────────────

func (r *sqliteTraefikComponentRepo) UpsertCerts(ctx context.Context, componentID string, certs []*models.TraefikComponentCert) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("upsert traefik component certs: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()
	for _, c := range certs {
		sansJSON, _ := json.Marshal(c.SANs)
		if c.ID == "" {
			c.ID = uuid.New().String()
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO traefik_component_certs
			  (id, component_id, domain, issuer, expires_at, sans, last_seen_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(component_id, domain) DO UPDATE SET
			  issuer       = excluded.issuer,
			  expires_at   = excluded.expires_at,
			  sans         = excluded.sans,
			  last_seen_at = excluded.last_seen_at`,
			c.ID, componentID, c.Domain, c.Issuer, c.ExpiresAt, string(sansJSON), now)
		if err != nil {
			return fmt.Errorf("upsert traefik component cert %s: %w", c.Domain, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("upsert traefik component certs: commit: %w", err)
	}
	return nil
}

func (r *sqliteTraefikComponentRepo) ListCerts(ctx context.Context, componentID string) ([]*models.TraefikComponentCert, error) {
	type row struct {
		ID          string     `db:"id"`
		ComponentID string     `db:"component_id"`
		Domain      string     `db:"domain"`
		Issuer      *string    `db:"issuer"`
		ExpiresAt   *time.Time `db:"expires_at"`
		SANsJSON    string     `db:"sans"`
		LastSeenAt  time.Time  `db:"last_seen_at"`
	}
	var rows []row
	err := r.db.SelectContext(ctx, &rows, `
		SELECT id, component_id, domain, issuer, expires_at,
		       COALESCE(sans,'[]') AS sans, last_seen_at
		FROM traefik_component_certs
		WHERE component_id = ?
		ORDER BY domain ASC`, componentID)
	if err != nil {
		return nil, fmt.Errorf("list traefik component certs: %w", err)
	}
	out := make([]*models.TraefikComponentCert, len(rows))
	for i, r := range rows {
		cert := &models.TraefikComponentCert{
			ID:          r.ID,
			ComponentID: r.ComponentID,
			Domain:      r.Domain,
			Issuer:      r.Issuer,
			ExpiresAt:   r.ExpiresAt,
			LastSeenAt:  r.LastSeenAt,
		}
		_ = json.Unmarshal([]byte(r.SANsJSON), &cert.SANs)
		out[i] = cert
	}
	return out, nil
}

// ── Routes ────────────────────────────────────────────────────────────────────

func (r *sqliteTraefikComponentRepo) UpsertRoutes(ctx context.Context, componentID string, routes []models.TraefikRoute) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("upsert traefik routes: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC().Format(time.RFC3339)
	for _, ro := range routes {
		if ro.ID == "" {
			ro.ID = uuid.New().String()
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO traefik_routes (id, component_id, name, rule, service, status, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(component_id, name) DO UPDATE SET
			  rule       = excluded.rule,
			  service    = excluded.service,
			  status     = excluded.status,
			  updated_at = excluded.updated_at`,
			ro.ID, componentID, ro.Name, ro.Rule, ro.Service, ro.Status, now)
		if err != nil {
			return fmt.Errorf("upsert traefik route %s: %w", ro.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("upsert traefik routes: commit: %w", err)
	}
	return nil
}

func (r *sqliteTraefikComponentRepo) ListRoutes(ctx context.Context, componentID string) ([]models.TraefikRoute, error) {
	var routes []models.TraefikRoute
	err := r.db.SelectContext(ctx, &routes, `
		SELECT id, component_id, name, rule, service, status, updated_at
		FROM traefik_routes
		WHERE component_id = ?
		ORDER BY name ASC`, componentID)
	if err != nil {
		return nil, fmt.Errorf("list traefik routes: %w", err)
	}
	if routes == nil {
		routes = []models.TraefikRoute{}
	}
	return routes, nil
}
