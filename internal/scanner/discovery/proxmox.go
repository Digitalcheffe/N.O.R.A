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

// ProxmoxDiscoveryScanner discovers VMs, LXC containers, and storage pools for
// a Proxmox VE infrastructure component.
type ProxmoxDiscoveryScanner struct {
	store *repo.Store
}

// NewProxmoxDiscoveryScanner returns a ProxmoxDiscoveryScanner backed by store.
func NewProxmoxDiscoveryScanner(store *repo.Store) *ProxmoxDiscoveryScanner {
	return &ProxmoxDiscoveryScanner{store: store}
}

// Discover fetches the current guest list from Proxmox, reconciles it against
// stored child InfrastructureComponent records, and writes discovery events.
func (s *ProxmoxDiscoveryScanner) Discover(ctx context.Context, entityID string, entityType string) (*scanner.DiscoveryResult, error) {
	c, err := s.store.InfraComponents.Get(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("get component %s: %w", entityID, err)
	}
	if c.Credentials == nil || *c.Credentials == "" {
		return nil, fmt.Errorf("no credentials configured for %s", c.Name)
	}

	poller, err := infra.NewProxmoxPoller(c.ID, *c.Credentials)
	if err != nil {
		return nil, fmt.Errorf("create proxmox poller: %w", err)
	}

	guests, err := poller.FetchGuests(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch guests: %w", err)
	}

	existing, err := s.store.InfraComponents.ListByParent(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("list children: %w", err)
	}

	// Build set of known child IDs.
	knownIDs := make(map[string]*models.InfrastructureComponent, len(existing))
	for i := range existing {
		knownIDs[existing[i].ID] = &existing[i]
	}

	// Build set of IDs currently returned by Proxmox.
	currentIDs := make(map[string]struct{}, len(guests))
	now := time.Now().UTC()
	polledAt := now.Format(time.RFC3339Nano)

	found := 0

	for _, g := range guests {
		childID := infra.ProxmoxChildID(entityID, g.VMID)
		currentIDs[childID] = struct{}{}

		status := "offline"
		if g.Status == "running" {
			status = "online"
		}

		existing, alreadyKnown := knownIDs[childID]
		if alreadyKnown {
			// Entity exists — check for name/status changes.
			changed := existing.Name != g.Name || existing.LastStatus != status
			if changed {
				existing.Name = g.Name
				if updateErr := s.store.InfraComponents.Update(ctx, existing); updateErr != nil {
					log.Printf("proxmox discovery: update child %s: %v", childID, updateErr)
				}
				if statusErr := s.store.InfraComponents.UpdateStatus(ctx, childID, status, polledAt); statusErr != nil {
					log.Printf("proxmox discovery: update status %s: %v", childID, statusErr)
				}
				writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "physical_host", "info",
					fmt.Sprintf("[discovery] %s updated: %s (status=%s)", g.GuestType, g.Name, status))
				found++
			} else {
				// Still present, no change — update status silently.
				_ = s.store.InfraComponents.UpdateStatus(ctx, childID, status, polledAt)
			}
		} else {
			// New entity — create child component.
			parentID := entityID
			child := &models.InfrastructureComponent{
				ID:               childID,
				Name:             g.Name,
				IP:               "",
				Type:             g.GuestType,
				CollectionMethod: "none",
				ParentID:         &parentID,
				Enabled:          true,
				LastStatus:       status,
				CreatedAt:        polledAt,
			}
			if createErr := s.store.InfraComponents.Create(ctx, child); createErr != nil {
				log.Printf("proxmox discovery: create child %s %q: %v", g.GuestType, g.Name, createErr)
				continue
			}
			writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "physical_host", "info",
				fmt.Sprintf("[discovery] New %s discovered: %s", g.GuestType, g.Name))
			found++
		}
	}

	// Mark missing children as offline.
	disappeared := 0
	for id, child := range knownIDs {
		if child.Type != "vm" && child.Type != "lxc" {
			continue
		}
		if _, stillPresent := currentIDs[id]; stillPresent {
			continue
		}
		if child.LastStatus == "offline" {
			continue // already marked
		}
		if updateErr := s.store.InfraComponents.UpdateStatus(ctx, id, "offline", polledAt); updateErr != nil {
			log.Printf("proxmox discovery: mark missing %s: %v", id, updateErr)
		}
		writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "physical_host", "warn",
			fmt.Sprintf("[discovery] Entity no longer found: %s", child.Name))
		disappeared++
	}

	if found == 0 && disappeared == 0 {
		writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "physical_host", "debug",
			fmt.Sprintf("[discovery] %s discovery completed — no changes", c.Name))
	}

	return &scanner.DiscoveryResult{
		EntityID:    entityID,
		EntityType:  entityType,
		Found:       found,
		Disappeared: disappeared,
	}, nil
}

// compile-time check that ProxmoxDiscoveryScanner satisfies the interface.
var _ scanner.DiscoveryScanner = (*ProxmoxDiscoveryScanner)(nil)
