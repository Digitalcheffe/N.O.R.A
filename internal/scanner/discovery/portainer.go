package discovery

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/internal/scanner"
)

// PortainerDiscoveryScanner verifies connectivity to a Portainer instance,
// lists its endpoints and containers, and writes discovery events.
type PortainerDiscoveryScanner struct {
	store *repo.Store
}

// NewPortainerDiscoveryScanner returns a PortainerDiscoveryScanner backed by store.
func NewPortainerDiscoveryScanner(store *repo.Store) *PortainerDiscoveryScanner {
	return &PortainerDiscoveryScanner{store: store}
}

// Discover connects to the Portainer API, enumerates endpoints and containers,
// writes events, and updates the component status.
func (s *PortainerDiscoveryScanner) Discover(ctx context.Context, entityID string, entityType string) (*scanner.DiscoveryResult, error) {
	c, err := s.store.InfraComponents.Get(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("get component %s: %w", entityID, err)
	}

	if c.Credentials == nil || *c.Credentials == "" {
		return nil, fmt.Errorf("no credentials configured for %q", c.Name)
	}

	creds, err := infra.ParsePortainerCredentials(*c.Credentials)
	if err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}

	client := infra.NewPortainerClient(creds.BaseURL, creds.APIKey)

	endpoints, err := client.ListEndpoints(ctx)
	if err != nil {
		_ = s.store.InfraComponents.UpdateStatus(ctx, entityID, "offline", time.Now().UTC().Format(time.RFC3339Nano))
		writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "portainer", "error",
			fmt.Sprintf("[discovery] Portainer unreachable — %s", err.Error()))
		return nil, fmt.Errorf("list endpoints: %w", err)
	}

	totalContainers := 0
	upserted := 0
	now := time.Now().UTC()

	for _, ep := range endpoints {
		containers, err := client.ListContainers(ctx, ep.ID)
		if err != nil {
			writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "portainer", "warn",
				fmt.Sprintf("[discovery] Endpoint %q unavailable — %s", ep.Name, err.Error()))
			continue
		}
		totalContainers += len(containers)

		for _, pc := range containers {
			name := pc.FirstName()
			if name == "" {
				continue
			}
			rec := &models.DiscoveredContainer{
				InfraComponentID: entityID,
				ContainerID:      pc.ID,
				ContainerName:    name,
				Image:            pc.Image,
				Status:           pc.State,
				LastSeenAt:       now,
				CreatedAt:        now,
			}
			if err := s.store.DiscoveredContainers.UpsertDiscoveredContainer(ctx, rec); err != nil {
				log.Printf("portainer discovery: upsert container %q: %v", name, err)
				continue
			}
			upserted++
		}
	}

	_ = s.store.InfraComponents.UpdateStatus(ctx, entityID, "online", time.Now().UTC().Format(time.RFC3339Nano))
	writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "portainer", "info",
		fmt.Sprintf("[discovery] Portainer sync complete — %d endpoint(s), %d container(s)", len(endpoints), totalContainers))

	return &scanner.DiscoveryResult{
		EntityID:   entityID,
		EntityType: entityType,
		Found:      upserted,
	}, nil
}
