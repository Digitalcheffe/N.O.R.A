package metrics

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/internal/scanner"
)

// PortainerMetricsScanner collects CPU% and memory% for every running container
// on each Portainer endpoint every MetricsInterval.  One scanner instance
// serves all portainer infrastructure components.
type PortainerMetricsScanner struct {
	store   *repo.Store
	tracker ThresholdTracker
}

// NewPortainerMetricsScanner returns a PortainerMetricsScanner backed by store.
func NewPortainerMetricsScanner(store *repo.Store) *PortainerMetricsScanner {
	return &PortainerMetricsScanner{
		store:   store,
		tracker: newThresholdTracker(),
	}
}

// CollectMetrics connects to the Portainer component identified by entityID,
// iterates all running containers across every endpoint, fetches stats, and
// writes cpu_percent and mem_percent resource_readings.
// Threshold events fire on CPU > 90% and memory > 90%.
func (s *PortainerMetricsScanner) CollectMetrics(ctx context.Context, entityID, entityType string) (*scanner.MetricsResult, error) {
	c, err := s.store.InfraComponents.Get(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("get component %s: %w", entityID, err)
	}
	if c.Credentials == nil || *c.Credentials == "" {
		return nil, fmt.Errorf("no credentials configured for %s", c.Name)
	}

	creds, err := infra.ParsePortainerCredentials(*c.Credentials)
	if err != nil {
		return nil, fmt.Errorf("parse portainer credentials: %w", err)
	}

	client := infra.NewPortainerClient(creds.BaseURL, creds.APIKey)

	endpoints, err := client.ListEndpoints(ctx)
	if err != nil {
		return nil, fmt.Errorf("list portainer endpoints: %w", err)
	}

	now := time.Now().UTC()
	readings := 0

	for _, ep := range endpoints {
		containers, err := client.ListContainers(ctx, ep.ID)
		if err != nil {
			log.Printf("portainer metrics: %s endpoint %d: list containers: %v", c.Name, ep.ID, err)
			continue
		}

		for _, ct := range containers {
			if ct.State != "running" {
				continue
			}

			name := ct.FirstName()
			if name == "" {
				name = ct.ID[:12]
			}

			stats, err := client.GetContainerStats(ctx, ep.ID, ct.ID)
			if err != nil {
				log.Printf("portainer metrics: %s/%s: stats: %v", c.Name, name, err)
				continue
			}

			cpuPct := infra.CalcCPUPercent(stats)
			memPct := infra.CalcMemPercent(stats)

			// Keyed by componentID/endpointID/containerName for uniqueness across endpoints.
			readingID := fmt.Sprintf("%s/%d/%s", entityID, ep.ID, name)

			writeReading(ctx, s.store, readingID, "portainer_container", "cpu_percent", cpuPct, now)
			writeReading(ctx, s.store, readingID, "portainer_container", "mem_percent", memPct, now)
			readings += 2

			displayName := fmt.Sprintf("%s/%s", c.Name, name)

			s.tracker.CheckAndFire(ctx, s.store, readingID, displayName, "docker_engine", "cpu_percent",
				cpuThreshold(cpuPct),
				func(l thresholdLevel) string {
					if l == levelNormal {
						return fmt.Sprintf("[metrics] CPU recovered — %s: %.1f%%", displayName, cpuPct)
					}
					return fmt.Sprintf("[metrics] High CPU — %s: %.1f%%", displayName, cpuPct)
				},
			)
			s.tracker.CheckAndFire(ctx, s.store, readingID, displayName, "docker_engine", "mem_percent",
				memThreshold(memPct),
				func(l thresholdLevel) string {
					if l == levelNormal {
						return fmt.Sprintf("[metrics] Memory recovered — %s: %.1f%%", displayName, memPct)
					}
					return fmt.Sprintf("[metrics] High memory — %s: %.1f%%", displayName, memPct)
				},
			)
		}
	}

	log.Printf("portainer metrics: %s: %d readings across %d endpoint(s)", c.Name, readings, len(endpoints))

	return &scanner.MetricsResult{
		EntityID:   entityID,
		EntityType: entityType,
		Readings:   readings,
	}, nil
}

// compile-time interface check.
var _ scanner.MetricsScanner = (*PortainerMetricsScanner)(nil)
