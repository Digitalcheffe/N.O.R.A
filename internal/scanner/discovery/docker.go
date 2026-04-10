package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/internal/scanner"
)

// DockerDiscoveryScanner discovers containers on a local Docker engine
// infrastructure component.
type DockerDiscoveryScanner struct {
	store *repo.Store
}

// NewDockerDiscoveryScanner returns a DockerDiscoveryScanner backed by store.
func NewDockerDiscoveryScanner(store *repo.Store) *DockerDiscoveryScanner {
	return &DockerDiscoveryScanner{store: store}
}

// Discover lists all containers (running and stopped) from the Docker daemon,
// reconciles them with discovered_containers, and writes discovery events.
func (s *DockerDiscoveryScanner) Discover(ctx context.Context, entityID string, entityType string) (*scanner.DiscoveryResult, error) {
	c, err := s.store.InfraComponents.Get(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("get component %s: %w", entityID, err)
	}

	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	defer cli.Close() //nolint:errcheck

	// List all containers (All=true includes stopped ones).
	containers, err := cli.ContainerList(ctx, dockercontainer.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}

	// Get previously known containers for this engine.
	known, err := s.store.DiscoveredContainers.ListDiscoveredContainers(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("list known containers: %w", err)
	}
	knownByID := make(map[string]*models.DiscoveredContainer, len(known))
	for _, dc := range known {
		knownByID[dc.ContainerID] = dc
	}

	encJSON := func(v any) *string {
		b, err := json.Marshal(v)
		if err != nil || string(b) == "null" {
			return nil
		}
		s := string(b)
		return &s
	}

	now := time.Now().UTC()
	found := 0
	runningIDs := make([]string, 0, len(containers))

	for _, ct := range containers {
		name := containerName(ct.Names)
		if name == "" {
			continue
		}
		status := "stopped"
		if ct.State == "running" {
			status = "running"
			runningIDs = append(runningIDs, ct.ID)
		}

		// Build network list matching the shared JSON schema.
		type netEntry struct {
			Name string `json:"name"`
			IP   string `json:"ip,omitempty"`
		}
		var nets []netEntry
		if ct.NetworkSettings != nil {
			for netName, ep := range ct.NetworkSettings.Networks {
				entry := netEntry{Name: netName}
				if ep != nil && ep.IPAddress != "" {
					entry.IP = ep.IPAddress
				}
				nets = append(nets, entry)
			}
		}

		dc := &models.DiscoveredContainer{
			InfraComponentID: entityID,
			SourceType:       "docker_engine",
			ContainerID:      ct.ID,
			ContainerName:    name,
			Image:            ct.Image,
			Status:           status,
			LastSeenAt:       now,
			CreatedAt:        now,
			Labels:           encJSON(ct.Labels),
			Ports:            encJSON(ct.Ports),
			Networks:         encJSON(nets),
			Volumes:          encJSON(ct.Mounts),
		}

		isNew := false
		if _, alreadyKnown := knownByID[ct.ID]; !alreadyKnown {
			isNew = true
		}

		if upsertErr := s.store.DiscoveredContainers.UpsertDiscoveredContainer(ctx, dc); upsertErr != nil {
			log.Printf("docker discovery: upsert %s: %v", name, upsertErr)
			continue
		}

		if isNew {
			writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "docker_engine", "info",
				fmt.Sprintf("[discovery] New container discovered: %s", name))
			found++
		}

		// Capture env vars via inspect (one extra call per container, non-fatal).
		if inspect, inspErr := cli.ContainerInspect(ctx, ct.ID); inspErr == nil {
			if len(inspect.Config.Env) > 0 {
				if b, jsonErr := json.Marshal(inspect.Config.Env); jsonErr == nil {
					ev := string(b)
					if updErr := s.store.DiscoveredContainers.UpdateContainerEnvVars(ctx, dc.ID, ev); updErr != nil {
						log.Printf("docker discovery: update env vars %s: %v", name, updErr)
					}
				}
			}
		} else {
			log.Printf("docker discovery: inspect %s: %v (non-fatal)", name, inspErr)
		}
	}

	// Mark containers no longer returned by Docker as stopped.
	disappeared := 0
	currentIDs := make(map[string]struct{}, len(containers))
	for _, ct := range containers {
		currentIDs[ct.ID] = struct{}{}
	}
	for _, dc := range known {
		if _, still := currentIDs[dc.ContainerID]; !still {
			if dc.Status != "stopped" {
				if updateErr := s.store.DiscoveredContainers.UpdateDiscoveredContainerStatus(ctx, dc.ID, "stopped", now); updateErr != nil {
					log.Printf("docker discovery: mark disappeared %s: %v", dc.ContainerName, updateErr)
				}
				writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "docker_engine", "warn",
					fmt.Sprintf("[discovery] Entity no longer found: %s", dc.ContainerName))
				disappeared++
			}
		}
	}

	if found == 0 && disappeared == 0 {
		writeDiscoveryEvent(ctx, s.store, entityID, c.Name, "docker_engine", "debug",
			fmt.Sprintf("[discovery] %s discovery completed — no changes", c.Name))
	}

	// Write container summary to meta.
	running := 0
	for _, ct := range containers {
		if ct.State == "running" {
			running++
		}
	}
	if metaBytes, jsonErr := json.Marshal(map[string]interface{}{
		"containers_running": running,
		"containers_stopped": len(containers) - running,
		"containers_total":   len(containers),
		"polled_at":          now.Format(time.RFC3339),
	}); jsonErr == nil {
		if updateErr := s.store.InfraComponents.UpdateMeta(ctx, entityID, string(metaBytes)); updateErr != nil {
			log.Printf("docker discovery: write meta for %s: %v", c.Name, updateErr)
		}
	}

	return &scanner.DiscoveryResult{
		EntityID:    entityID,
		EntityType:  entityType,
		Found:       found,
		Disappeared: disappeared,
	}, nil
}

// containerName strips the leading "/" Docker prepends to container names.
func containerName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
}

// compile-time check.
var _ scanner.DiscoveryScanner = (*DockerDiscoveryScanner)(nil)
