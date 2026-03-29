package models

import "time"

// InfrastructureComponent is the unified model for every infrastructure entity
// (Proxmox nodes, Synology, VMs, LXC containers, bare-metal, Windows hosts, etc.).
type InfrastructureComponent struct {
	ID               string  `db:"id"                 json:"id"`
	Name             string  `db:"name"               json:"name"`
	IP               string  `db:"ip"                 json:"ip"`
	Type             string  `db:"type"               json:"type"`
	CollectionMethod string  `db:"collection_method"  json:"collection_method"`
	ParentID         *string `db:"parent_id"          json:"parent_id,omitempty"`
	Credentials      *string `db:"credentials"        json:"-"` // never serialised to API response
	SNMPConfig       *string `db:"snmp_config"        json:"snmp_config,omitempty"`
	Notes            string  `db:"notes"              json:"notes"`
	Enabled          bool    `db:"enabled"            json:"enabled"`
	LastPolledAt     *string `db:"last_polled_at"     json:"last_polled_at,omitempty"`
	LastStatus       string  `db:"last_status"        json:"last_status"`
	CreatedAt        string  `db:"created_at"         json:"created_at"`
}

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

// TraefikComponentCert is a TLS certificate discovered from a traefik-type
// infrastructure component and linked to it for cascade-delete.
type TraefikComponentCert struct {
	ID          string     `db:"id"           json:"id"`
	ComponentID string     `db:"component_id" json:"component_id"`
	Domain      string     `db:"domain"       json:"domain"`
	Issuer      *string    `db:"issuer"       json:"issuer,omitempty"`
	ExpiresAt   *time.Time `db:"expires_at"   json:"expires_at,omitempty"`
	SANs        []string   `db:"-"            json:"sans"`
	SANsJSON    string     `db:"sans"         json:"-"`
	LastSeenAt  time.Time  `db:"last_seen_at" json:"last_seen_at"`
}

// TraefikRoute is an HTTP router entry discovered from a traefik-type component.
type TraefikRoute struct {
	ID          string `db:"id"           json:"id"`
	ComponentID string `db:"component_id" json:"component_id"`
	Name        string `db:"name"         json:"name"`
	Rule        string `db:"rule"         json:"rule"`
	Service     string `db:"service"      json:"service"`
	Status      string `db:"status"       json:"status"`
	UpdatedAt   string `db:"updated_at"   json:"updated_at"`
}
