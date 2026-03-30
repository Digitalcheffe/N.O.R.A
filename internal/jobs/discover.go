package jobs

import (
	"context"
	"fmt"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/internal/scanner"
	"github.com/digitalcheffe/nora/internal/scanner/discovery"
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

	sc := discoveryScanner(store, c)
	if sc == nil {
		return nil, fmt.Errorf("no discovery scanner for type=%q method=%q", c.Type, c.CollectionMethod)
	}

	return sc.Discover(ctx, c.ID, c.Type)
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
	}
	// Fallback: collection_method-based dispatch.
	if c.CollectionMethod == "snmp" {
		return discovery.NewSNMPDiscoveryScanner(store)
	}
	return nil
}
