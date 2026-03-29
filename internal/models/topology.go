package models

import "time"

// DockerEngine represents a Docker daemon accessible to NORA.
// InfraComponentID links to the parent infrastructure_components row (optional).
type DockerEngine struct {
	ID               string    `db:"id"                  json:"id"`
	InfraComponentID string    `db:"infra_component_id"  json:"infra_component_id,omitempty"`
	Name             string    `db:"name"                json:"name"`
	SocketType       string    `db:"socket_type"         json:"socket_type"`
	SocketPath       string    `db:"socket_path"         json:"socket_path"`
	CreatedAt        time.Time `db:"created_at"          json:"created_at"`
}
