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

// SynologyMetricsScanner collects CPU%, memory, temperature, and volume
// utilisation metrics from Synology DSM every MetricsInterval.
type SynologyMetricsScanner struct {
	store   *repo.Store
	pollers sync.Map // componentID → *infra.SynologyPoller
	tracker ThresholdTracker
}

// NewSynologyMetricsScanner returns a SynologyMetricsScanner backed by store.
func NewSynologyMetricsScanner(store *repo.Store) *SynologyMetricsScanner {
	return &SynologyMetricsScanner{
		store:   store,
		tracker: newThresholdTracker(),
	}
}

// CollectMetrics fetches Synology metrics and writes them to resource_readings.
// Fires threshold events on CPU > 90%, memory > 90%, temperature > 80°C / > 90°C.
func (s *SynologyMetricsScanner) CollectMetrics(ctx context.Context, entityID string, entityType string) (*scanner.MetricsResult, error) {
	c, err := s.store.InfraComponents.Get(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("get component %s: %w", entityID, err)
	}
	if c.Credentials == nil || *c.Credentials == "" {
		return nil, fmt.Errorf("no credentials configured for %s", c.Name)
	}

	poller, err := s.getOrCreatePoller(entityID, *c.Credentials)
	if err != nil {
		return nil, fmt.Errorf("create synology poller: %w", err)
	}

	snap, err := poller.CollectMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("collect metrics: %w", err)
	}

	now := time.Now().UTC()
	readings := 0

	for _, m := range []struct {
		metric string
		value  float64
	}{
		{"cpu_percent", snap.CPUPercent},
		{"mem_percent", snap.MemPercent},
		{"mem_used_gb", snap.MemUsedGB},
		{"mem_total_gb", snap.MemTotalGB},
		{"temperature_c", float64(snap.TempC)},
		{"uptime_seconds", float64(snap.UptimeSecs)},
	} {
		writeReading(ctx, s.store, entityID, "synology", m.metric, m.value, now)
		readings++
	}

	for _, vol := range snap.VolumeStats {
		writeReading(ctx, s.store, entityID, "synology", "disk_percent_"+vol.Key, vol.DiskPercent, now)
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
	s.tracker.CheckAndFire(ctx, s.store, entityID, c.Name, "physical_host", "temperature_c",
		tempThreshold(float64(snap.TempC)),
		func(l thresholdLevel) string {
			switch l {
			case levelError:
				return fmt.Sprintf("[metrics] Critical temperature — %s: %d°C", c.Name, snap.TempC)
			case levelWarn:
				return fmt.Sprintf("[metrics] High temperature — %s: %d°C", c.Name, snap.TempC)
			default:
				return fmt.Sprintf("[metrics] Temperature recovered — %s: %d°C", c.Name, snap.TempC)
			}
		},
	)

	log.Printf("synology metrics: %s: %d readings (cpu=%.1f%% mem=%.1f%% temp=%d°C)",
		c.Name, readings, snap.CPUPercent, snap.MemPercent, snap.TempC)

	return &scanner.MetricsResult{
		EntityID:   entityID,
		EntityType: entityType,
		Readings:   readings,
	}, nil
}

func (s *SynologyMetricsScanner) getOrCreatePoller(componentID, credJSON string) (*infra.SynologyPoller, error) {
	if v, ok := s.pollers.Load(componentID); ok {
		return v.(*infra.SynologyPoller), nil
	}
	p, err := infra.NewSynologyPoller(componentID, credJSON)
	if err != nil {
		return nil, err
	}
	s.pollers.Store(componentID, p)
	return p, nil
}

// compile-time interface check.
var _ scanner.MetricsScanner = (*SynologyMetricsScanner)(nil)
