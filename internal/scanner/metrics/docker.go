package metrics

import (
	"context"
	"fmt"
	"log"

	"github.com/digitalcheffe/nora/internal/docker"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/internal/scanner"
)

// DockerMetricsScanner collects CPU% and memory metrics from all running
// containers on a Docker engine. It delegates to docker.ResourcePoller which
// already implements stable source IDs, threshold events, and per-container
// readings. One scanner instance is created per docker_engine component.
type DockerMetricsScanner struct {
	store   *repo.Store
	poller  *docker.ResourcePoller
	engineID string
}

// NewDockerMetricsScanner returns a DockerMetricsScanner that uses the given
// ResourcePoller. The engineID must match the infrastructure_components.id of
// the docker_engine component so source IDs are derived consistently.
func NewDockerMetricsScanner(store *repo.Store, engineID string, poller *docker.ResourcePoller) *DockerMetricsScanner {
	return &DockerMetricsScanner{
		store:    store,
		poller:   poller,
		engineID: engineID,
	}
}

// CollectMetrics calls ResourcePoller.PollAll to collect CPU%, memory used,
// and memory% for all running containers. Threshold events are emitted by the
// ResourcePoller itself; the MetricsScanner does not add additional ones.
func (s *DockerMetricsScanner) CollectMetrics(ctx context.Context, entityID string, entityType string) (*scanner.MetricsResult, error) {
	if s.poller == nil {
		return nil, fmt.Errorf("docker resource poller not available for %s", entityID)
	}

	s.poller.PollAll(ctx)
	log.Printf("docker metrics: %s: poll complete", entityID)

	// ResourcePoller.PollAll writes directly to resource_readings. We don't
	// have a reading count from it, so return a sentinel value of 1 to signal
	// that at least one pass completed successfully.
	return &scanner.MetricsResult{
		EntityID:   entityID,
		EntityType: entityType,
		Readings:   1,
	}, nil
}

// compile-time interface check.
var _ scanner.MetricsScanner = (*DockerMetricsScanner)(nil)
