package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/google/uuid"

	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// discoveryListAPI is the minimal Docker client subset needed for the initial
// container scan, enabling mock injection in tests.
type discoveryListAPI interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
}

// Discoverer writes container discovery records into discovered_containers and
// runs profile matching on every upsert.
type Discoverer struct {
	store    *repo.Store
	registry *apptemplate.Registry
	engineID string
	client   discoveryListAPI
}

// NewDiscoverer creates a Discoverer connected to the local Docker daemon.
// It returns an error only if the Docker client cannot be constructed.
func NewDiscoverer(store *repo.Store, registry *apptemplate.Registry, engineID string) (*Discoverer, error) {
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &Discoverer{store: store, registry: registry, engineID: engineID, client: cli}, nil
}

// newDiscovererWithClient creates a Discoverer with an injected client (for tests).
func newDiscovererWithClient(store *repo.Store, registry *apptemplate.Registry, engineID string, cli discoveryListAPI) *Discoverer {
	return &Discoverer{store: store, registry: registry, engineID: engineID, client: cli}
}

// containerEnrichment carries the optional metadata extracted from a
// container.Summary (available during ScanAll, not during event-driven upserts).
type containerEnrichment struct {
	Ports           *string
	Labels          *string
	Volumes         *string
	Networks        *string
	DockerCreatedAt *time.Time
}

// extractEnrichment converts a container.Summary into a containerEnrichment.
// JSON encoding errors are silently ignored — the fields are informational only.
func extractEnrichment(c container.Summary) *containerEnrichment {
	enc := func(v any) *string {
		b, err := json.Marshal(v)
		if err != nil || string(b) == "null" {
			return nil
		}
		s := string(b)
		return &s
	}

	var dockerCreatedAt *time.Time
	if c.Created > 0 {
		t := time.Unix(c.Created, 0).UTC()
		dockerCreatedAt = &t
	}

	// Collect networks with their container IPs.
	type networkEntry struct {
		Name string `json:"name"`
		IP   string `json:"ip,omitempty"`
	}
	var networks []networkEntry
	if c.NetworkSettings != nil {
		for name, ep := range c.NetworkSettings.Networks {
			entry := networkEntry{Name: name}
			if ep != nil && ep.IPAddress != "" {
				entry.IP = ep.IPAddress
			}
			networks = append(networks, entry)
		}
	}

	return &containerEnrichment{
		Ports:           enc(c.Ports),
		Labels:          enc(c.Labels),
		Volumes:         enc(c.Mounts),
		Networks:        enc(networks),
		DockerCreatedAt: dockerCreatedAt,
	}
}

// ScanAll lists all running containers from the Docker daemon and upserts each
// one into discovered_containers. Called once at NORA startup and whenever a
// manual scan is triggered. After upserting running containers it reconciles
// any previously-discovered containers that are no longer running.
func (d *Discoverer) ScanAll(ctx context.Context) {
	log.Printf("docker discovery: scanning all running containers for engine %s", d.engineID)

	polledAt := time.Now().UTC().Format(time.RFC3339Nano)
	containers, err := d.client.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		log.Printf("docker discovery: list containers: %v", err)
		if updateErr := d.store.InfraComponents.UpdateStatus(ctx, d.engineID, "offline", polledAt); updateErr != nil {
			log.Printf("docker discovery: update status offline: %v", updateErr)
		}
		return
	}

	runningIDs := make([]string, 0, len(containers))
	for _, c := range containers {
		name := containerNameFrom(c.Names)
		image := c.Image
		meta := extractEnrichment(c)
		if err := d.upsert(ctx, c.ID, name, image, "running", meta); err != nil {
			log.Printf("docker discovery: upsert %s: %v", name, err)
		}
		runningIDs = append(runningIDs, c.ID)
	}

	// Mark any previously-discovered containers that are no longer running as
	// stopped so they don't show as running in the UI after a restart or removal.
	if err := d.store.DiscoveredContainers.MarkStoppedIfNotRunning(ctx, d.engineID, runningIDs); err != nil {
		log.Printf("docker discovery: mark stopped: %v", err)
	}

	if updateErr := d.store.InfraComponents.UpdateStatus(ctx, d.engineID, "online", polledAt); updateErr != nil {
		log.Printf("docker discovery: update status online: %v", updateErr)
	}

	log.Printf("docker discovery: initial scan complete — %d containers", len(containers))
}

// HandleEvent is the hook wired into the Watcher. It upserts the container
// into discovered_containers and runs profile matching.
// status is one of: running | stopped | exited
func (d *Discoverer) HandleEvent(ctx context.Context, containerID, name, image, status string) {
	if err := d.upsert(ctx, containerID, name, image, status, nil); err != nil {
		log.Printf("docker discovery: upsert event for %s: %v", name, err)
	}
}

// upsert writes or updates a discovered_containers record and runs profile matching.
// The unique key is (infra_component_id, container_name): rebuilding a container
// produces a new Docker hash but the same name, so the existing row is updated in
// place and the app_id link is preserved automatically by the DB.
// meta is optional enrichment from container.Summary; pass nil when not available.
func (d *Discoverer) upsert(ctx context.Context, containerID, name, image, status string, meta *containerEnrichment) error {
	now := time.Now().UTC()

	dc := &models.DiscoveredContainer{
		InfraComponentID: d.engineID,
		SourceType:       "docker_engine",
		ContainerID:      containerID,
		ContainerName:    name,
		Image:            image,
		Status:           status,
		LastSeenAt:       now,
		CreatedAt:        now,
	}

	if meta != nil {
		dc.Ports = meta.Ports
		dc.Labels = meta.Labels
		dc.Volumes = meta.Volumes
		dc.Networks = meta.Networks
		dc.DockerCreatedAt = meta.DockerCreatedAt
	}

	// Run profile matching and attach suggestion if confidence is sufficient.
	if match := MatchContainerToProfile(name, image, d.registry); match != nil {
		dc.ProfileSuggestion = &match.ProfileID
		dc.SuggestionConfidence = &match.Confidence
	}

	if err := d.store.DiscoveredContainers.UpsertDiscoveredContainer(ctx, dc); err != nil {
		return err
	}

	// Register the parent relationship in component_links so the topology and
	// Relationships UI can traverse docker_engine → container edges.
	if err := d.store.ComponentLinks.SetParent(ctx, "docker_engine", d.engineID, "container", dc.ID); err != nil {
		log.Printf("docker discovery: set parent link for container %s: %v", name, err)
	}
	return nil
}

// containerNameFrom returns the primary container name from the Docker names
// slice, stripping the leading "/" that Docker prepends.
func containerNameFrom(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
}

// EnsureLocalInfraComponent looks up the first infrastructure_components record
// with type="docker_engine" and collection_method="docker_socket". If none
// exists, it creates one and returns its ID.
// This is used at startup to ensure the local Docker socket watcher has an
// infrastructure component record to associate discovered containers with.
func EnsureLocalInfraComponent(ctx context.Context, store *repo.Store) (string, error) {
	components, err := store.InfraComponents.List(ctx)
	if err != nil {
		return "", fmt.Errorf("list infrastructure components: %w", err)
	}

	for _, c := range components {
		if c.Type == "docker_engine" && c.CollectionMethod == "docker_socket" {
			return c.ID, nil
		}
	}

	// No local docker engine component found — create one.
	now := time.Now().UTC().Format(time.RFC3339)
	comp := &models.InfrastructureComponent{
		ID:               uuid.New().String(),
		Name:             "Local Docker",
		IP:               "",
		Type:             "docker_engine",
		CollectionMethod: "docker_socket",
		Notes:            "",
		Enabled:          true,
		LastStatus:       "unknown",
		CreatedAt:        now,
	}
	if err := store.InfraComponents.Create(ctx, comp); err != nil {
		return "", fmt.Errorf("create local docker infra component: %w", err)
	}
	log.Printf("docker discovery: created local docker engine component %s", comp.ID)
	return comp.ID, nil
}
