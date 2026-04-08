package models

// InfrastructureComponent is the unified model for every infrastructure entity
// (Proxmox nodes, Synology, VMs, LXC containers, bare-metal, Windows hosts, etc.).
type InfrastructureComponent struct {
	ID               string  `db:"id"                 json:"id"`
	Name             string  `db:"name"               json:"name"`
	IP               string  `db:"ip"                 json:"ip"`
	Type             string  `db:"type"               json:"type"`
	CollectionMethod string  `db:"collection_method"  json:"collection_method"`
	Credentials      *string `db:"credentials"        json:"-"` // never serialised to API response
	SNMPConfig       *string `db:"snmp_config"        json:"snmp_config,omitempty"`
	// Meta holds the latest poller snapshot as JSON (SNMP, Synology, Traefik, etc.).
	// Written by each type-specific poller; never returned directly in API responses.
	Meta             *string `db:"meta"               json:"-"`
	Notes            string  `db:"notes"              json:"notes"`
	Enabled          bool    `db:"enabled"            json:"enabled"`
	LastPolledAt     *string `db:"last_polled_at"     json:"last_polled_at,omitempty"`
	LastStatus       string  `db:"last_status"        json:"last_status"`
	CreatedAt        string  `db:"created_at"         json:"created_at"`
}




