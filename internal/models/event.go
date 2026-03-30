package models

import "time"

// Event represents a unified event from any NORA source: app webhooks, infra
// pollers, monitor checks, Docker watchers, or internal system activity.
//
// source_type values: app | physical_host | virtual_host | docker_engine |
//
//	monitor_check | system
type Event struct {
	ID         string    `db:"id"          json:"id"`
	Level      string    `db:"level"       json:"level"`
	SourceName string    `db:"source_name" json:"source_name"`
	SourceType string    `db:"source_type" json:"source_type"`
	SourceID   string    `db:"source_id"   json:"source_id"`
	Title      string    `db:"title"       json:"title"`
	Payload    string    `db:"payload"     json:"payload"`
	CreatedAt  time.Time `db:"created_at"  json:"created_at"`
}
