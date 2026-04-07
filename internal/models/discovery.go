package models

import "time"

// DiscoveredContainer is a container found by polling a Docker Engine or Portainer component.
// app_id is set when the user links the container to an NORA app.
// profile_suggestion is the profile_id NORA matched via image/name heuristics.
// source_type records which kind of component discovered this container ("docker_engine" | "portainer").
// The parent-child relationship (source component → container) is stored in component_links.
type DiscoveredContainer struct {
	ID                   string     `db:"id"                    json:"id"`
	InfraComponentID     string     `db:"infra_component_id"    json:"infra_component_id"`
	SourceType           string     `db:"source_type"           json:"source_type"`
	ContainerID          string     `db:"container_id"          json:"container_id"`
	ContainerName        string     `db:"container_name"        json:"container_name"`
	Image                string     `db:"image"                 json:"image"`
	Status               string     `db:"status"                json:"status"`
	AppID                *string    `db:"app_id"                json:"app_id,omitempty"`
	ProfileSuggestion    *string    `db:"profile_suggestion"    json:"profile_suggestion,omitempty"`
	SuggestionConfidence *int       `db:"suggestion_confidence" json:"suggestion_confidence,omitempty"`
	LastSeenAt           time.Time  `db:"last_seen_at"          json:"last_seen_at"`
	CreatedAt            time.Time  `db:"created_at"            json:"created_at"`
	// Fields added in migration 022 (DD-9).
	ImageDigest          *string    `db:"image_digest"           json:"image_digest,omitempty"`
	RegistryDigest       *string    `db:"registry_digest"        json:"registry_digest,omitempty"`
	ImageUpdateAvailable int        `db:"image_update_available" json:"image_update_available"`
	ImageLastCheckedAt   *time.Time `db:"image_last_checked_at"  json:"image_last_checked_at,omitempty"`
	// Fields added in migration 037 (AP-04).
	Ports           *string    `db:"ports"             json:"ports,omitempty"`             // JSON array of port bindings
	Labels          *string    `db:"labels"            json:"labels,omitempty"`            // JSON map of container labels
	Volumes         *string    `db:"volumes"           json:"volumes,omitempty"`           // JSON array of mount points
	Networks        *string    `db:"networks"          json:"networks,omitempty"`          // JSON array of network names
	RestartPolicy   *string    `db:"restart_policy"    json:"restart_policy,omitempty"`   // e.g. "always", "no"
	DockerCreatedAt *time.Time `db:"docker_created_at" json:"docker_created_at,omitempty"` // when Docker created the container
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
	// Service health — populated from Traefik /api/http/services during discovery.
	ServiceStatus *string `db:"service_status" json:"service_status,omitempty"`
	ServiceType   *string `db:"service_type"   json:"service_type,omitempty"`
	ServersTotal  int     `db:"servers_total"  json:"servers_total"`
	ServersUp     int     `db:"servers_up"     json:"servers_up"`
	ServersDown   int     `db:"servers_down"   json:"servers_down"`
}
