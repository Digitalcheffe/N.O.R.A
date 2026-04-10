package jobs

import (
	"context"
	"fmt"
	"log"

	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/internal/scanner"
	"github.com/digitalcheffe/nora/internal/scanner/discovery"
	"github.com/digitalcheffe/nora/internal/scanner/snapshot"
)

// DiscoverOneComponent immediately runs the discovery scanner for a single
// infrastructure component.  It is the backend for the
// POST /infrastructure/{id}/discover API endpoint.
//
// The scanner implementation is selected based on the component's type and
// collection_method, mirroring the logic in scanner.ScanScheduler.
func DiscoverOneComponent(ctx context.Context, store *repo.Store, c *models.InfrastructureComponent) (*scanner.DiscoveryResult, error) {
	if !c.Enabled {
		return nil, fmt.Errorf("component is disabled")
	}

	// Portainer: route through PortainerEnrichmentWorker rather than
	// PortainerDiscoveryScanner.  The enrichment worker correctly sets
	// source_type="portainer", calls MarkStoppedIfNotRunning, performs digest
	// extraction, and emits image-update events — the discovery scanner does
	// none of those things.
	if c.Type == "portainer" {
		worker := infra.NewPortainerEnrichmentWorker(store)
		if err := worker.Run(ctx); err != nil {
			return nil, err
		}
		return &scanner.DiscoveryResult{EntityID: c.ID, EntityType: c.Type}, nil
	}

	sc := discoveryScanner(store, c)
	if sc == nil {
		return nil, fmt.Errorf("no discovery scanner for type=%q method=%q", c.Type, c.CollectionMethod)
	}

	result, err := sc.Discover(ctx, c.ID, c.Type)

	// For Traefik components also run the snapshot scanner so the services table
	// is populated alongside the routes — both use the same API connection.
	if c.Type == "traefik" {
		if _, snapErr := snapshot.NewTraefikSnapshotScanner(store).TakeSnapshot(ctx, c.ID, c.Type); snapErr != nil {
			log.Printf("discover: traefik snapshot for %s (%s): %v", c.Name, c.ID, snapErr)
		}
	}

	return result, err
}

// discoveryScanner returns the correct DiscoveryScanner for c, or nil if none
// applies.  Type is checked first; collection_method is the fallback.
func discoveryScanner(store *repo.Store, c *models.InfrastructureComponent) scanner.DiscoveryScanner {
	switch c.Type {
	case "proxmox_node":
		return discovery.NewProxmoxDiscoveryScanner(store)
	case "docker_engine":
		return discovery.NewDockerDiscoveryScanner(store)
	case "synology":
		return discovery.NewSynologyDiscoveryScanner(store)
	case "opnsense":
		return discovery.NewOPNsenseDiscoveryScanner(store)
	case "portainer":
		return discovery.NewPortainerDiscoveryScanner(store)
	case "traefik":
		return discovery.NewTraefikDiscoveryScanner(store)
	}
	// Fallback: collection_method-based dispatch.
	if c.CollectionMethod == "snmp" {
		return discovery.NewSNMPDiscoveryScanner(store)
	}
	return nil
}
