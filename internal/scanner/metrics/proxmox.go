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

// ProxmoxMetricsScanner collects CPU%, memory, and disk metrics from Proxmox
// VE nodes every MetricsInterval. One scanner instance serves all
// proxmox_node infrastructure components.
type ProxmoxMetricsScanner struct {
	store    *repo.Store
	pollers  sync.Map // componentID → *infra.ProxmoxPoller
	tracker  ThresholdTracker
}

// NewProxmoxMetricsScanner returns a ProxmoxMetricsScanner backed by store.
func NewProxmoxMetricsScanner(store *repo.Store) *ProxmoxMetricsScanner {
	return &ProxmoxMetricsScanner{
		store:   store,
		tracker: newThresholdTracker(),
	}
}

// CollectMetrics fetches node metrics for entityID and writes them to
// resource_readings. It fires threshold events on CPU > 90%, memory > 90%.
func (s *ProxmoxMetricsScanner) CollectMetrics(ctx context.Context, entityID string, entityType string) (*scanner.MetricsResult, error) {
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

	nodeMetrics, err := poller.CollectNodeMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("collect node metrics: %w", err)
	}

	now := time.Now().UTC()
	readings := 0

	for _, nm := range nodeMetrics {
		// Use a per-node sub-key so multi-node clusters have distinct readings.
		nodeID := entityID + "/" + nm.Node

		for _, m := range []struct {
			metric string
			value  float64
		}{
			{"cpu_percent", nm.CPUPercent},
			{"mem_percent", nm.MemPercent},
			{"mem_used_gb", nm.MemUsedGB},
			{"mem_total_gb", nm.MemTotalGB},
			{"disk_percent", nm.DiskPercent},
		} {
			writeReading(ctx, s.store, nodeID, "proxmox_node", m.metric, m.value, now)
			readings++
		}

		// Threshold checks (per spec: CPU>90%→warn, mem>90%→warn)
		s.tracker.CheckAndFire(ctx, s.store, nodeID, c.Name, "physical_host", "cpu_percent",
			cpuThreshold(nm.CPUPercent),
			func(l thresholdLevel) string {
				if l == levelNormal {
					return fmt.Sprintf("[metrics] CPU recovered — %s/%s: %.1f%%", c.Name, nm.Node, nm.CPUPercent)
				}
				return fmt.Sprintf("[metrics] High CPU — %s/%s: %.1f%%", c.Name, nm.Node, nm.CPUPercent)
			},
		)
		s.tracker.CheckAndFire(ctx, s.store, nodeID, c.Name, "physical_host", "mem_percent",
			memThreshold(nm.MemPercent),
			func(l thresholdLevel) string {
				if l == levelNormal {
					return fmt.Sprintf("[metrics] Memory recovered — %s/%s: %.1f%%", c.Name, nm.Node, nm.MemPercent)
				}
				return fmt.Sprintf("[metrics] High memory — %s/%s: %.1f%%", c.Name, nm.Node, nm.MemPercent)
			},
		)
	}

	if len(nodeMetrics) > 0 {
		log.Printf("proxmox metrics: %s: %d readings from %d node(s)", c.Name, readings, len(nodeMetrics))
	}

	return &scanner.MetricsResult{
		EntityID:   entityID,
		EntityType: entityType,
		Readings:   readings,
	}, nil
}

func (s *ProxmoxMetricsScanner) getOrCreatePoller(componentID, credJSON string) (*infra.ProxmoxPoller, error) {
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
var _ scanner.MetricsScanner = (*ProxmoxMetricsScanner)(nil)
