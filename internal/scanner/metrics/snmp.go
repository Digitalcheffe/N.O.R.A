package metrics

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

// SNMPMetricsScanner collects CPU%, memory%, and per-disk utilisation from
// hosts using SNMP. It is registered by collection_method="snmp" rather than
// by entity type, so it covers physical_host and any other type that uses SNMP.
type SNMPMetricsScanner struct {
	store   *repo.Store
	pollers sync.Map // componentID → *infra.SNMPPoller
	tracker ThresholdTracker
}

// NewSNMPMetricsScanner returns an SNMPMetricsScanner backed by store.
func NewSNMPMetricsScanner(store *repo.Store) *SNMPMetricsScanner {
	return &SNMPMetricsScanner{
		store:   store,
		tracker: newThresholdTracker(),
	}
}

// CollectMetrics reads CPU%, memory, disk, and uptime via SNMP and writes them
// to resource_readings. Fires threshold events on CPU > 90%, memory > 90%.
func (s *SNMPMetricsScanner) CollectMetrics(ctx context.Context, entityID string, entityType string) (*scanner.MetricsResult, error) {
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
		return nil, fmt.Errorf("collect metrics: %w", err)
	}

	now := time.Now().UTC()
	readings := 0

	writeReading(ctx, s.store, entityID, "snmp_host", "cpu_percent", snap.CPUPercent, now)
	readings++
	writeReading(ctx, s.store, entityID, "snmp_host", "mem_percent", snap.MemPercent, now)
	readings++
	writeReading(ctx, s.store, entityID, "snmp_host", "mem_used_gb", snap.MemUsedGB, now)
	readings++
	writeReading(ctx, s.store, entityID, "snmp_host", "mem_total_gb", snap.MemTotalGB, now)
	readings++

	for _, d := range snap.Disks {
		label := infra.SanitizeDiskLabel(d.Label)
		writeReading(ctx, s.store, entityID, "snmp_host", "disk_percent_"+label, d.Percent, now)
		readings++
	}

	// Threshold checks
	s.tracker.CheckAndFire(ctx, s.store, entityID, c.Name, "physical_host", "cpu_percent",
		cpuThreshold(snap.CPUPercent),
		func(l thresholdLevel) string {
			if l == levelNormal {
				return fmt.Sprintf("[metrics] CPU recovered — %s: %.1f%%", c.Name, snap.CPUPercent)
			}
			return fmt.Sprintf("[metrics] High CPU — %s: %.1f%%", c.Name, snap.CPUPercent)
		},
	)
	s.tracker.CheckAndFire(ctx, s.store, entityID, c.Name, "physical_host", "mem_percent",
		memThreshold(snap.MemPercent),
		func(l thresholdLevel) string {
			if l == levelNormal {
				return fmt.Sprintf("[metrics] Memory recovered — %s: %.1f%%", c.Name, snap.MemPercent)
			}
			return fmt.Sprintf("[metrics] High memory — %s: %.1f%%", c.Name, snap.MemPercent)
		},
	)

	log.Printf("snmp metrics: %s: %d readings (cpu=%.1f%% mem=%.1f%%)",
		c.Name, readings, snap.CPUPercent, snap.MemPercent)

	return &scanner.MetricsResult{
		EntityID:   entityID,
		EntityType: entityType,
		Readings:   readings,
	}, nil
}

func (s *SNMPMetricsScanner) getOrCreatePoller(componentID, ip, cfgJSON string) (*infra.SNMPPoller, error) {
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
var _ scanner.MetricsScanner = (*SNMPMetricsScanner)(nil)
