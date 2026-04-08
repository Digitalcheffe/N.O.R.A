package discovery

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/internal/scanner"
)

// TraefikDiscoveryScanner discovers routes for traefik infrastructure components
// and refreshes the traefik_component_routes and discovered_routes tables.
// One instance serves all traefik components.
type TraefikDiscoveryScanner struct {
	store *repo.Store
}

// NewTraefikDiscoveryScanner returns a TraefikDiscoveryScanner backed by store.
func NewTraefikDiscoveryScanner(store *repo.Store) *TraefikDiscoveryScanner {
	return &TraefikDiscoveryScanner{store: store}
}

// Discover pings the Traefik API, fetches all HTTP routers, upserts the route
// cache, and runs a full TraefikDiscovery pass to populate discovered_routes
// (including container cross-referencing and Host() rule parsing).
func (s *TraefikDiscoveryScanner) Discover(ctx context.Context, entityID, entityType string) (*scanner.DiscoveryResult, error) {
	c, err := s.store.InfraComponents.Get(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("get component %s: %w", entityID, err)
	}

	apiURL, apiKey := infra.ResolveTraefikCreds(c.IP, c.Credentials)
	client := infra.NewTraefikClient(apiURL, apiKey)

	if err := client.Ping(ctx); err != nil {
		polledAt := time.Now().UTC().Format(time.RFC3339Nano)
		_ = s.store.InfraComponents.UpdateStatus(ctx, entityID, "offline", polledAt)
		return nil, fmt.Errorf("ping: %w", err)
	}

	// Populate discovered_routes — cross-references containers, parses Host() rules,
	// enriches with service health. This is the single source of truth for routes.
	disc := infra.NewTraefikDiscovery(s.store)
	if err := disc.Run(ctx, c); err != nil {
		log.Printf("traefik discovery: discovered_routes for %s: %v", c.Name, err)
	}

	rawRouters, err := client.FetchRouters(ctx)
	found := 0
	if err != nil {
		log.Printf("traefik discovery: fetch routers for %s: %v", c.Name, err)
	} else {
		found = len(rawRouters)
	}

	polledAt := time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.store.InfraComponents.UpdateStatus(ctx, entityID, "online", polledAt); err != nil {
		log.Printf("traefik discovery: update status for %s: %v", c.Name, err)
	}

	writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "traefik", "debug",
		fmt.Sprintf("[discovery] Traefik %s: %d routes discovered", c.Name, found))

	return &scanner.DiscoveryResult{
		EntityID:   entityID,
		EntityType: entityType,
		Found:      found,
	}, nil
}

// compile-time check that TraefikDiscoveryScanner satisfies the interface.
var _ scanner.DiscoveryScanner = (*TraefikDiscoveryScanner)(nil)
