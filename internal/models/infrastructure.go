package models

import "time"

// InfraIntegration represents a connected infrastructure provider (e.g. Traefik).
type InfraIntegration struct {
	ID           string     `db:"id"             json:"id"`
	Type         string     `db:"type"           json:"type"`           // "traefik"
	Name         string     `db:"name"           json:"name"`
	APIURL       string     `db:"api_url"        json:"api_url"`
	APIKey       *string    `db:"api_key"        json:"api_key,omitempty"`
	Enabled      bool       `db:"enabled"        json:"enabled"`
	LastSyncedAt *time.Time `db:"last_synced_at" json:"last_synced_at,omitempty"`
	LastStatus   *string    `db:"last_status"    json:"last_status,omitempty"`   // "ok" | "error"
	LastError    *string    `db:"last_error"     json:"last_error,omitempty"`
	CreatedAt    time.Time  `db:"created_at"     json:"created_at"`
}

// TraefikCert is a TLS certificate discovered via the Traefik API and cached locally.
type TraefikCert struct {
	ID            string     `db:"id"             json:"id"`
	IntegrationID string     `db:"integration_id" json:"integration_id"`
	Domain        string     `db:"domain"         json:"domain"`
	Issuer        *string    `db:"issuer"         json:"issuer,omitempty"`
	ExpiresAt     *time.Time `db:"expires_at"     json:"expires_at,omitempty"`
	SANs          []string   `db:"-"              json:"sans"`
	SANsJSON      string     `db:"sans"           json:"-"` // raw JSON column
	LastSeenAt    time.Time  `db:"last_seen_at"   json:"last_seen_at"`
}
