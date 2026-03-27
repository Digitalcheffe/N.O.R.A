package models

import "time"

// PhysicalHost represents a physical machine (bare metal or Proxmox node).
type PhysicalHost struct {
	ID        string    `db:"id"         json:"id"`
	Name      string    `db:"name"       json:"name"`
	IP        string    `db:"ip"         json:"ip"`
	Type      string    `db:"type"       json:"type"`
	Notes     string    `db:"notes"      json:"notes"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// VirtualHost represents a VM, LXC container, or WSL instance.
type VirtualHost struct {
	ID             string    `db:"id"               json:"id"`
	PhysicalHostID string    `db:"physical_host_id" json:"physical_host_id,omitempty"`
	Name           string    `db:"name"             json:"name"`
	IP             string    `db:"ip"               json:"ip"`
	Type           string    `db:"type"             json:"type"`
	CreatedAt      time.Time `db:"created_at"       json:"created_at"`
}

// DockerEngine represents a Docker daemon accessible to NORA.
type DockerEngine struct {
	ID            string    `db:"id"              json:"id"`
	VirtualHostID string    `db:"virtual_host_id" json:"virtual_host_id,omitempty"`
	Name          string    `db:"name"            json:"name"`
	SocketType    string    `db:"socket_type"     json:"socket_type"`
	SocketPath    string    `db:"socket_path"     json:"socket_path"`
	CreatedAt     time.Time `db:"created_at"      json:"created_at"`
}
