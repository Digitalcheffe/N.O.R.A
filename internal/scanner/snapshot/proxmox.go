package snapshot

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/internal/scanner"
)

// ProxmoxSnapshotScanner captures storage pool utilisation, PVE version, and
// available update counts for all nodes in a Proxmox cluster every
// SnapshotInterval.
type ProxmoxSnapshotScanner struct {
	store   *repo.Store
	pollers sync.Map // componentID → *infra.ProxmoxPoller
}

// NewProxmoxSnapshotScanner returns a ProxmoxSnapshotScanner backed by store.
func NewProxmoxSnapshotScanner(store *repo.Store) *ProxmoxSnapshotScanner {
	return &ProxmoxSnapshotScanner{store: store}
}

// TakeSnapshot fetches storage pool utilisation, PVE version, and update counts
// for every node in the Proxmox cluster and writes snapshot rows. Events are
// fired on condition changes only.
func (s *ProxmoxSnapshotScanner) TakeSnapshot(ctx context.Context, entityID string, entityType string) (*scanner.SnapshotResult, error) {
	c, err := s.store.InfraComponents.Get(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("get component %s: %w", entityID, err)
	}
	if c.Credentials == nil || *c.Credentials == "" {
		return nil, fmt.Errorf("no credentials configured for %s", c.Name)
	}

	poller, err := s.getOrCreatePoller(entityID, *c.Credentials)
	if err != nil {
		return nil, fmt.Errorf("create proxmox poller: %w", err)
	}

	now := time.Now().UTC()
	changed := false

	// ── Storage pools ─────────────────────────────────────────────────────────
	pools, err := poller.FetchStoragePools(ctx)
	if err != nil {
		log.Printf("proxmox snapshot: %s: fetch storage pools: %v (non-fatal)", c.Name, err)
	} else {
		for _, pool := range pools {
			if !pool.Active {
				continue
			}
			nodePoolKey := fmt.Sprintf("%s/%s", pool.Node, pool.Name)
			pctStr := fmt.Sprintf("%.2f", pool.UsedPercent)

			_, ch := captureSnapshot(ctx, s.store, "physical_host", entityID,
				"storage_pct_"+nodePoolKey, pctStr, now)
			if ch {
				changed = true
				newCond := storageCondition(pool.UsedPercent)
				if newCond != "ok" {
					level, title := storageEventTitle(c.Name, nodePoolKey, newCond, pool.UsedPercent)
					writeSnapshotEvent(ctx, s.store, entityID, c.Name, "physical_host", level, title)
				}
			}

			captureSnapshot(ctx, s.store, "physical_host", entityID,
				"storage_used_bytes_"+nodePoolKey, fmt.Sprintf("%d", pool.UsedBytes), now)
			captureSnapshot(ctx, s.store, "physical_host", entityID,
				"storage_total_bytes_"+nodePoolKey, fmt.Sprintf("%d", pool.TotalBytes), now)
		}
	}

	// ── Node status: PVE version + available updates ───────────────────────────
	nodeStatuses, err := poller.FetchNodeStatus(ctx)
	if err != nil {
		log.Printf("proxmox snapshot: %s: fetch node status: %v (non-fatal)", c.Name, err)
	} else {
		for _, ns := range nodeStatuses {
			// PVE version
			if ns.PVEVersion != "" {
				_, ch := captureSnapshot(ctx, s.store, "physical_host", entityID,
					"pve_version_"+ns.Node, ns.PVEVersion, now)
				if ch {
					changed = true
					writeSnapshotEvent(ctx, s.store, entityID, c.Name, "physical_host", "info",
						fmt.Sprintf("[snapshot] PVE updated — %s/%s: %s", c.Name, ns.Node, ns.PVEVersion))
				}
			}

			// Available updates
			updStr := fmt.Sprintf("%d", ns.UpdatesAvailable)
			prevUpd, ch := captureSnapshot(ctx, s.store, "physical_host", entityID,
				"updates_available_"+ns.Node, updStr, now)
			if ch {
				changed = true
				prev := 0
				fmt.Sscanf(prevUpd, "%d", &prev)
				if ns.UpdatesAvailable > 0 && prev == 0 {
					writeSnapshotEvent(ctx, s.store, entityID, c.Name, "physical_host", "info",
						fmt.Sprintf("[snapshot] %d update(s) available — %s/%s",
							ns.UpdatesAvailable, c.Name, ns.Node))
				} else if ns.UpdatesAvailable == 0 && prev > 0 {
					writeSnapshotEvent(ctx, s.store, entityID, c.Name, "physical_host", "info",
						fmt.Sprintf("[snapshot] Updates applied — %s/%s", c.Name, ns.Node))
				}
			}
		}
	}

	writeDebugEvent(ctx, s.store, entityID, c.Name, "physical_host")
	log.Printf("proxmox snapshot: %s: done (changed=%v)", c.Name, changed)

	return &scanner.SnapshotResult{
		EntityID:   entityID,
		EntityType: entityType,
		Changed:    changed,
	}, nil
}

func (s *ProxmoxSnapshotScanner) getOrCreatePoller(componentID, credJSON string) (*infra.ProxmoxPoller, error) {
	if v, ok := s.pollers.Load(componentID); ok {
		return v.(*infra.ProxmoxPoller), nil
	}
	p, err := infra.NewProxmoxPoller(componentID, credJSON)
	if err != nil {
		return nil, err
	}
	s.pollers.Store(componentID, p)
	return p, nil
}

// compile-time interface check.
var _ scanner.SnapshotScanner = (*ProxmoxSnapshotScanner)(nil)
