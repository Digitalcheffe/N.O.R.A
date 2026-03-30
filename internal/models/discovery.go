package models

import "time"

// DiscoveredContainer is a container found by polling a Docker Engine component.
// app_id is set when the user links the container to an NORA app.
// profile_suggestion is the profile_id NORA matched via image/name heuristics.
type DiscoveredContainer struct {
	ID                   string    `db:"id"                    json:"id"`
	InfraComponentID     string    `db:"infra_component_id"    json:"infra_component_id"`
	ContainerID          string    `db:"container_id"          json:"container_id"`
	ContainerName        string    `db:"container_name"        json:"container_name"`
	Image                string    `db:"image"                 json:"image"`
	Status               string    `db:"status"                json:"status"`
	AppID                *string   `db:"app_id"                json:"app_id,omitempty"`
	ProfileSuggestion    *string   `db:"profile_suggestion"    json:"profile_suggestion,omitempty"`
	SuggestionConfidence *int      `db:"suggestion_confidence" json:"suggestion_confidence,omitempty"`
	LastSeenAt           time.Time `db:"last_seen_at"          json:"last_seen_at"`
	CreatedAt            time.Time `db:"created_at"            json:"created_at"`
}

// DiscoveredRoute is an HTTP router entry found via a Traefik infrastructure component.
// container_id is auto-linked when the backend service name matches a known container.
// app_id is set when the user links the route to an NORA app.
type DiscoveredRoute struct {
	ID               string     `db:"id"                json:"id"`
	InfrastructureID string     `db:"infrastructure_id" json:"infrastructure_id"`
	RouterName       string     `db:"router_name"       json:"router_name"`
	Rule             string     `db:"rule"              json:"rule"`
	Domain           *string    `db:"domain"            json:"domain,omitempty"`
	BackendService   *string    `db:"backend_service"   json:"backend_service,omitempty"`
	ContainerID      *string    `db:"container_id"      json:"container_id,omitempty"`
	AppID            *string    `db:"app_id"            json:"app_id,omitempty"`
	SSLExpiry        *time.Time `db:"ssl_expiry"        json:"ssl_expiry,omitempty"`
	SSLIssuer        *string    `db:"ssl_issuer"        json:"ssl_issuer,omitempty"`
	LastSeenAt       time.Time  `db:"last_seen_at"      json:"last_seen_at"`
	CreatedAt        time.Time  `db:"created_at"        json:"created_at"`
	// Fields added in migration 017 (Infra-10).
	RouterStatus     string  `db:"router_status"      json:"router_status"`
	Provider         *string `db:"provider"           json:"provider,omitempty"`
	EntryPoints      *string `db:"entry_points"       json:"entry_points,omitempty"` // JSON array
	HasTLSResolver   int     `db:"has_tls_resolver"   json:"has_tls_resolver"`
	CertResolverName *string `db:"cert_resolver_name" json:"cert_resolver_name,omitempty"`
	ServiceName      *string `db:"service_name"       json:"service_name,omitempty"`
}
