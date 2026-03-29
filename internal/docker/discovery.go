package docker

import (
	"context"
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

// ScanAll lists all running containers from the Docker daemon and upserts each
// one into discovered_containers. Called once at NORA startup.
func (d *Discoverer) ScanAll(ctx context.Context) {
	log.Printf("docker discovery: scanning all running containers for engine %s", d.engineID)

	containers, err := d.client.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		log.Printf("docker discovery: list containers: %v", err)
		return
	}

	for _, c := range containers {
		name := containerNameFrom(c.Names)
		image := c.Image
		if err := d.upsert(ctx, c.ID, name, image, "running"); err != nil {
			log.Printf("docker discovery: upsert %s: %v", name, err)
		}
	}

	log.Printf("docker discovery: initial scan complete — %d containers", len(containers))
}

// HandleEvent is the hook wired into the Watcher. It upserts the container
// into discovered_containers and runs profile matching.
// status is one of: running | stopped | exited
func (d *Discoverer) HandleEvent(ctx context.Context, containerID, name, image, status string) {
	if err := d.upsert(ctx, containerID, name, image, status); err != nil {
		log.Printf("docker discovery: upsert event for %s: %v", name, err)
	}
}

// upsert writes or updates a discovered_containers record and runs profile matching.
func (d *Discoverer) upsert(ctx context.Context, containerID, name, image, status string) error {
	now := time.Now().UTC()

	dc := &models.DiscoveredContainer{
		DockerEngineID: d.engineID,
		ContainerID:    containerID,
		ContainerName:  name,
		Image:          image,
		Status:         status,
		LastSeenAt:     now,
		CreatedAt:      now,
	}

	// Run profile matching and attach suggestion if confidence is sufficient.
	if match := MatchContainerToProfile(name, image, d.registry); match != nil {
		dc.ProfileSuggestion = &match.ProfileID
		dc.SuggestionConfidence = &match.Confidence
	}

	return d.store.DiscoveredContainers.UpsertDiscoveredContainer(ctx, dc)
}

// containerNameFrom returns the primary container name from the Docker names
// slice, stripping the leading "/" that Docker prepends.
func containerNameFrom(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
}

// EnsureLocalEngine looks up the first docker_engine record with
// socket_type="local". If none exists, it creates one and returns its ID.
// This is used at startup to ensure the local Docker socket watcher has an
// engine record to associate discovered containers with.
func EnsureLocalEngine(ctx context.Context, store *repo.Store) (string, error) {
	engines, err := store.DockerEngines.List(ctx)
	if err != nil {
		return "", fmt.Errorf("list docker engines: %w", err)
	}

	for _, e := range engines {
		if e.SocketType == "local" {
			return e.ID, nil
		}
	}

	// No local engine found — create one.
	engine := &models.DockerEngine{
		ID:         uuid.New().String(),
		Name:       "Local Docker",
		SocketType: "local",
		SocketPath: "/var/run/docker.sock",
	}
	if err := store.DockerEngines.Create(ctx, engine); err != nil {
		return "", fmt.Errorf("create local docker engine: %w", err)
	}
	log.Printf("docker discovery: created local engine record %s", engine.ID)
	return engine.ID, nil
}
