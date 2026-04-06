package models

import "time"

// DockerEngine represents a Docker daemon accessible to NORA.
// Parent relationships are stored in component_links.
type DockerEngine struct {
	ID         string    `db:"id"          json:"id"`
	Name       string    `db:"name"        json:"name"`
	SocketType string    `db:"socket_type" json:"socket_type"`
	SocketPath string    `db:"socket_path" json:"socket_path"`
	CreatedAt  time.Time `db:"created_at"  json:"created_at"`
}
