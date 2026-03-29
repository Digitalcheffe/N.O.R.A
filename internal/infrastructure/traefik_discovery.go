// Package infrastructure provides Traefik and other infrastructure discovery workers.
package infrastructure

import (
	"context"
	"encoding/json"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

// hostRuleRE matches Host(`domain`) or Host("domain") in a Traefik rule string.
// It does NOT match HostRegexp(...) because after "Host" we require "(" immediately.
var hostRuleRE = regexp.MustCompile("Host\\(`([^`]+)`\\)|Host\\(\"([^\"]+)\"\\)")

// ParseHostFromRule extracts the first hostname from a Traefik routing rule.
// Returns nil for PathPrefix-only rules, HostRegexp rules, and empty/unrecognised input.
//
// Supported forms:
//
//	Host(`sonarr.example.com`)  → "sonarr.example.com"
//	Host("sonarr.example.com")  → "sonarr.example.com"
//	HostRegexp(...)             → nil  (too ambiguous)
//	PathPrefix(`/api`)          → nil  (no Host present)
//	Host(`a.com`) && PathPrefix → "a.com"  (first Host match wins)
func ParseHostFromRule(rule string) *string {
	m := hostRuleRE.FindStringSubmatch(rule)
	if m == nil {
		return nil
	}
	var domain string
	if m[1] != "" {
		domain = m[1]
	} else {
		domain = m[2]
	}
	if domain == "" {
		return nil
	}
	return &domain
}

// traefikDiscoveryCredentials is the JSON shape stored in
// infrastructure_components.credentials for traefik-type components.
type traefikDiscoveryCredentials struct {
	APIURL string `json:"api_url"`
	APIKey string `json:"api_key"`
}

// TraefikDiscovery polls the Traefik HTTP router API for an infrastructure
// component and upserts entries into discovered_routes, cross-referencing
// backend service names against known discovered_containers.
type TraefikDiscovery struct {
	store *repo.Store
}

// NewTraefikDiscovery returns a TraefikDiscovery wired to store.
func NewTraefikDiscovery(store *repo.Store) *TraefikDiscovery {
	return &TraefikDiscovery{store: store}
}

// Run fetches HTTP routers from the Traefik API for the given component and
// upserts the results into discovered_routes. SSL data is sourced from the
// traefik_component_certs cache that was populated earlier in the same poll
// cycle. Container cross-referencing is done by matching the Traefik service
// name against discovered_containers.container_name.
//
// Non-fatal errors (SSL lookup failures, individual upsert failures) are logged
// and execution continues so that a single bad router does not abort the sync.
func (t *TraefikDiscovery) Run(ctx context.Context, component *models.InfrastructureComponent) error {
	if component.Credentials == nil || *component.Credentials == "" {
		return nil
	}

	var creds traefikDiscoveryCredentials
	if err := json.Unmarshal([]byte(*component.Credentials), &creds); err != nil {
		return nil // malformed credentials — already logged upstream
	}
	if creds.APIURL == "" {
		return nil
	}

	client := infra.NewTraefikClient(creds.APIURL, creds.APIKey)

	// ── Fetch routers from Traefik ────────────────────────────────────────────

	routers, err := client.FetchRouters(ctx)
	if err != nil {
		return err
	}

	// ── Build lookup maps ─────────────────────────────────────────────────────

	// container_name → discovered_containers.id (UUID PK)
	containerByName := make(map[string]string)
	containers, err := t.store.DiscoveredContainers.ListAllDiscoveredContainers(ctx)
	if err != nil {
		log.Printf("traefik discovery: list containers for cross-ref: %v", err)
	} else {
		for _, c := range containers {
			containerByName[c.ContainerName] = c.ID
		}
	}

	// domain → (ssl_expiry, ssl_issuer) from traefik_component_certs
	type sslInfo struct {
		expiry *time.Time
		issuer *string
	}
	sslByDomain := make(map[string]sslInfo)
	certs, err := t.store.TraefikComponents.ListCerts(ctx, component.ID)
	if err != nil {
		log.Printf("traefik discovery: list certs for ssl lookup: %v", err)
	} else {
		for _, cert := range certs {
			sslByDomain[cert.Domain] = sslInfo{
				expiry: cert.ExpiresAt,
				issuer: cert.Issuer,
			}
		}
	}

	// ── Upsert discovered routes ──────────────────────────────────────────────

	now := time.Now().UTC()
	for _, rr := range routers {
		domain := ParseHostFromRule(rr.Rule)

		// Strip Traefik provider suffix (e.g. "sonarr@docker" → "sonarr").
		backendService := rr.ServiceName
		if idx := strings.Index(backendService, "@"); idx >= 0 {
			backendService = backendService[:idx]
		}

		var containerIDPtr *string
		if cid, ok := containerByName[backendService]; ok {
			containerIDPtr = &cid
		}

		var sslExpiry *time.Time
		var sslIssuer *string
		if domain != nil {
			if info, ok := sslByDomain[*domain]; ok {
				sslExpiry = info.expiry
				sslIssuer = info.issuer
			}
		}

		var domainPtr *string
		if domain != nil {
			domainPtr = domain
		}
		var backendPtr *string
		if backendService != "" {
			backendPtr = &backendService
		}

		route := &models.DiscoveredRoute{
			ID:               uuid.New().String(),
			InfrastructureID: component.ID,
			RouterName:       rr.Name,
			Rule:             rr.Rule,
			Domain:           domainPtr,
			BackendService:   backendPtr,
			ContainerID:      containerIDPtr,
			SSLExpiry:        sslExpiry,
			SSLIssuer:        sslIssuer,
			LastSeenAt:       now,
			CreatedAt:        now,
		}

		if err := t.store.DiscoveredRoutes.UpsertDiscoveredRoute(ctx, route); err != nil {
			log.Printf("traefik discovery: upsert route %s: %v", rr.Name, err)
		}
	}

	log.Printf("traefik discovery: upserted %d routes for component %s (%s)",
		len(routers), component.Name, component.ID)
	return nil
}
