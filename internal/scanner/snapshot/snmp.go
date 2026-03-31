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

// SNMPSnapshotScanner captures per-disk utilisation for hosts polled via SNMP
// every SnapshotInterval. It fires warn events when a disk exceeds 80% and
// error events when it exceeds 90%, and recovery events when it drops back below
// the threshold.
//
// It is registered by collection_method="snmp" rather than by entity type,
// matching the pattern established by the SNMP metrics scanner.
type SNMPSnapshotScanner struct {
	store   *repo.Store
	pollers sync.Map // componentID → *infra.SNMPPoller
}

// NewSNMPSnapshotScanner returns an SNMPSnapshotScanner backed by store.
func NewSNMPSnapshotScanner(store *repo.Store) *SNMPSnapshotScanner {
	return &SNMPSnapshotScanner{store: store}
}

// TakeSnapshot reads disk utilisation from the SNMP host and writes snapshot
// rows. Events fire on condition changes only.
func (s *SNMPSnapshotScanner) TakeSnapshot(ctx context.Context, entityID string, entityType string) (*scanner.SnapshotResult, error) {
	c, err := s.store.InfraComponents.Get(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("get component %s: %w", entityID, err)
	}
	if c.SNMPConfig == nil || *c.SNMPConfig == "" {
		return nil, fmt.Errorf("no SNMP config for %s", c.Name)
	}

	poller, err := s.getOrCreatePoller(entityID, c.IP, *c.SNMPConfig)
	if err != nil {
		return nil, fmt.Errorf("create SNMP poller: %w", err)
	}

	snap, err := poller.CollectMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("collect SNMP metrics: %w", err)
	}

	now := time.Now().UTC()
	changed := false

	for _, disk := range snap.Disks {
		label := infra.SanitizeDiskLabel(disk.Label)
		pctStr := fmt.Sprintf("%.2f", disk.Percent)
		prevStr, ch := captureSnapshot(ctx, s.store, "snmp_host", entityID,
			"disk_pct_"+label, pctStr, now)
		if ch {
			changed = true
			newCond := storageCondition(disk.Percent)
			var prevPct float64
			fmt.Sscanf(prevStr, "%f", &prevPct)
			prevCond := storageCondition(prevPct)

			if newCond != prevCond {
				level, title := storageEventTitle(c.Name, disk.Label, newCond, disk.Percent)
				writeSnapshotEvent(ctx, s.store, entityID, c.Name, "physical_host", level, title)
			}
		}
	}

	writeDebugEvent(ctx, s.store, entityID, c.Name, "physical_host")
	log.Printf("snmp snapshot: %s: done (changed=%v, disks=%d)", c.Name, changed, len(snap.Disks))

	return &scanner.SnapshotResult{
		EntityID:   entityID,
		EntityType: entityType,
		Changed:    changed,
	}, nil
}

func (s *SNMPSnapshotScanner) getOrCreatePoller(componentID, ip, cfgJSON string) (*infra.SNMPPoller, error) {
	if v, ok := s.pollers.Load(componentID); ok {
		return v.(*infra.SNMPPoller), nil
	}
	p, err := infra.NewSNMPPoller(componentID, ip, cfgJSON)
	if err != nil {
		return nil, err
	}
	s.pollers.Store(componentID, p)
	return p, nil
}

// compile-time interface check.
var _ scanner.SnapshotScanner = (*SNMPSnapshotScanner)(nil)
