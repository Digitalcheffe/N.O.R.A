package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// InfraRepo defines CRUD operations for infrastructure integrations and the
// Traefik cert cache.
type InfraRepo interface {
	List(ctx context.Context) ([]*models.InfraIntegration, error)
	Get(ctx context.Context, id string) (*models.InfraIntegration, error)
	Create(ctx context.Context, i *models.InfraIntegration) error
	Update(ctx context.Context, i *models.InfraIntegration) error
	Delete(ctx context.Context, id string) error

	// UpsertCerts replaces the cert cache entries for integrationID with certs.
	UpsertCerts(ctx context.Context, integrationID string, certs []*models.TraefikCert) error
	// ListCerts returns all cached certs for integrationID.
	ListCerts(ctx context.Context, integrationID string) ([]*models.TraefikCert, error)
	// GetCertByDomain returns the most recently seen cert for domain across all integrations.
	GetCertByDomain(ctx context.Context, domain string) (*models.TraefikCert, error)
}

type sqliteInfraRepo struct {
	db *sqlx.DB
}

// NewInfraRepo returns an InfraRepo backed by the given SQLite database.
func NewInfraRepo(db *sqlx.DB) InfraRepo {
	return &sqliteInfraRepo{db: db}
}

// ── InfraIntegration CRUD ────────────────────────────────────────────────────

func (r *sqliteInfraRepo) List(ctx context.Context) ([]*models.InfraIntegration, error) {
	var rows []*models.InfraIntegration
	err := r.db.SelectContext(ctx, &rows, `
		SELECT id, type, name, api_url, api_key, enabled,
		       last_synced_at, last_status, last_error, created_at
		FROM infrastructure_integrations
		ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("list integrations: %w", err)
	}
	if rows == nil {
		rows = []*models.InfraIntegration{}
	}
	return rows, nil
}

func (r *sqliteInfraRepo) Get(ctx context.Context, id string) (*models.InfraIntegration, error) {
	var row models.InfraIntegration
	err := r.db.GetContext(ctx, &row, `
		SELECT id, type, name, api_url, api_key, enabled,
		       last_synced_at, last_status, last_error, created_at
		FROM infrastructure_integrations WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get integration: %w", err)
	}
	return &row, nil
}

func (r *sqliteInfraRepo) Create(ctx context.Context, i *models.InfraIntegration) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO infrastructure_integrations
		  (id, type, name, api_url, api_key, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		i.ID, i.Type, i.Name, i.APIURL, i.APIKey, i.Enabled, i.CreatedAt)
	if err != nil {
		return fmt.Errorf("create integration: %w", err)
	}
	return nil
}

func (r *sqliteInfraRepo) Update(ctx context.Context, i *models.InfraIntegration) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE infrastructure_integrations
		SET type=?, name=?, api_url=?, api_key=?, enabled=?,
		    last_synced_at=?, last_status=?, last_error=?
		WHERE id=?`,
		i.Type, i.Name, i.APIURL, i.APIKey, i.Enabled,
		i.LastSyncedAt, i.LastStatus, i.LastError,
		i.ID)
	if err != nil {
		return fmt.Errorf("update integration: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteInfraRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM infrastructure_integrations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete integration: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ── Traefik cert cache ───────────────────────────────────────────────────────

func (r *sqliteInfraRepo) UpsertCerts(ctx context.Context, integrationID string, certs []*models.TraefikCert) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("upsert certs: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now().UTC()
	for _, c := range certs {
		sansJSON, _ := json.Marshal(c.SANs)
		if c.ID == "" {
			c.ID = uuid.New().String()
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO traefik_certs (id, integration_id, domain, issuer, expires_at, sans, last_seen_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(integration_id, domain) DO UPDATE SET
			  issuer       = excluded.issuer,
			  expires_at   = excluded.expires_at,
			  sans         = excluded.sans,
			  last_seen_at = excluded.last_seen_at`,
			c.ID, integrationID, c.Domain, c.Issuer, c.ExpiresAt, string(sansJSON), now)
		if err != nil {
			return fmt.Errorf("upsert cert %s: %w", c.Domain, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("upsert certs: commit: %w", err)
	}
	return nil
}

func (r *sqliteInfraRepo) ListCerts(ctx context.Context, integrationID string) ([]*models.TraefikCert, error) {
	type row struct {
		ID            string     `db:"id"`
		IntegrationID string     `db:"integration_id"`
		Domain        string     `db:"domain"`
		Issuer        *string    `db:"issuer"`
		ExpiresAt     *time.Time `db:"expires_at"`
		SANsJSON      string     `db:"sans"`
		LastSeenAt    time.Time  `db:"last_seen_at"`
	}
	var rows []row
	err := r.db.SelectContext(ctx, &rows, `
		SELECT id, integration_id, domain, issuer, expires_at,
		       COALESCE(sans,'[]') AS sans, last_seen_at
		FROM traefik_certs
		WHERE integration_id = ?
		ORDER BY domain ASC`, integrationID)
	if err != nil {
		return nil, fmt.Errorf("list certs: %w", err)
	}
	out := make([]*models.TraefikCert, len(rows))
	for i, r := range rows {
		cert := &models.TraefikCert{
			ID:            r.ID,
			IntegrationID: r.IntegrationID,
			Domain:        r.Domain,
			Issuer:        r.Issuer,
			ExpiresAt:     r.ExpiresAt,
			LastSeenAt:    r.LastSeenAt,
		}
		_ = json.Unmarshal([]byte(r.SANsJSON), &cert.SANs)
		out[i] = cert
	}
	return out, nil
}

func (r *sqliteInfraRepo) GetCertByDomain(ctx context.Context, domain string) (*models.TraefikCert, error) {
	type row struct {
		ID            string     `db:"id"`
		IntegrationID string     `db:"integration_id"`
		Domain        string     `db:"domain"`
		Issuer        *string    `db:"issuer"`
		ExpiresAt     *time.Time `db:"expires_at"`
		SANsJSON      string     `db:"sans"`
		LastSeenAt    time.Time  `db:"last_seen_at"`
	}
	var r2 row
	err := r.db.GetContext(ctx, &r2, `
		SELECT id, integration_id, domain, issuer, expires_at,
		       COALESCE(sans,'[]') AS sans, last_seen_at
		FROM traefik_certs
		WHERE domain = ?
		ORDER BY last_seen_at DESC
		LIMIT 1`, domain)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get cert by domain: %w", err)
	}
	cert := &models.TraefikCert{
		ID:            r2.ID,
		IntegrationID: r2.IntegrationID,
		Domain:        r2.Domain,
		Issuer:        r2.Issuer,
		ExpiresAt:     r2.ExpiresAt,
		LastSeenAt:    r2.LastSeenAt,
	}
	_ = json.Unmarshal([]byte(r2.SANsJSON), &cert.SANs)
	return cert, nil
}
