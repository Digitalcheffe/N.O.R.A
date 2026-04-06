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

// ProxmoxDiscoveryScanner discovers VMs and storage pools for
// a Proxmox VE infrastructure component. LXC containers are not tracked.
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

		compType := proxmoxOSTypeToComponentType(g.OSType)

		existing, alreadyKnown := knownIDs[childID]
		if alreadyKnown {
			// Entity exists — check for name, status, or type changes.
			changed := existing.Name != g.Name || existing.LastStatus != status || existing.Type != compType
			if changed {
				existing.Name = g.Name
				existing.Type = compType
				if updateErr := s.store.InfraComponents.Update(ctx, existing); updateErr != nil {
					log.Printf("proxmox discovery: update child %s: %v", childID, updateErr)
				}
				if statusErr := s.store.InfraComponents.UpdateStatus(ctx, childID, status, polledAt); statusErr != nil {
					log.Printf("proxmox discovery: update status %s: %v", childID, statusErr)
				}
				writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "physical_host", "info",
					fmt.Sprintf("[discovery] vm updated: %s (status=%s, type=%s)", g.Name, status, compType))
				found++
			} else {
				// Still present, no change — update status silently.
				_ = s.store.InfraComponents.UpdateStatus(ctx, childID, status, polledAt)
			}
		} else {
			// New entity — create child component.
			child := &models.InfrastructureComponent{
				ID:               childID,
				Name:             g.Name,
				IP:               g.IP,
				Type:             compType,
				CollectionMethod: "none",
				Enabled:          true,
				LastStatus:       status,
				CreatedAt:        polledAt,
			}
			if createErr := s.store.InfraComponents.Create(ctx, child); createErr != nil {
				log.Printf("proxmox discovery: create child vm %q: %v", g.Name, createErr)
				continue
			}
			if linkErr := s.store.ComponentLinks.SetParent(ctx, "proxmox_node", entityID, compType, childID); linkErr != nil {
				log.Printf("proxmox discovery: set parent link for vm %q: %v", g.Name, linkErr)
			}
			writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "physical_host", "info",
				fmt.Sprintf("[discovery] New vm discovered: %s (type=%s)", g.Name, compType))
			found++
		}
	}

	// Mark missing VM children as offline.
	disappeared := 0
	for id, child := range knownIDs {
		if child.Type != "vm_linux" && child.Type != "vm_windows" && child.Type != "vm_other" {
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

// proxmoxOSTypeToComponentType maps a Proxmox ostype string to an
// infrastructure_components type value.
func proxmoxOSTypeToComponentType(ostype string) string {
	switch ostype {
	case "l24", "l26", "debian", "ubuntu", "centos", "fedora",
		"opensuse", "archlinux", "gentoo", "alpine", "nixos":
		return "vm_linux"
	case "win10", "win11", "win7", "win8", "wxp",
		"w2k", "w2k3", "w2k8", "w2k19", "w2k22":
		return "vm_windows"
	default:
		return "vm_other"
	}
}

// compile-time check that ProxmoxDiscoveryScanner satisfies the interface.
var _ scanner.DiscoveryScanner = (*ProxmoxDiscoveryScanner)(nil)
