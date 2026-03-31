package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/google/uuid"
)

// PortainerEnrichmentWorker polls all enabled Portainer infrastructure components
// every 15 minutes. For each Portainer endpoint, it lists containers, matches them
// to NORA-known discovered_containers records by name, updates image_update_available,
// and emits an event on false→true transitions.
//
// Gate: if no infrastructure components with type="portainer" are configured,
// each Run call returns early. Docker Engine components are NOT required.
type PortainerEnrichmentWorker struct {
	store *repo.Store

	// lastUpdateAvailable tracks the last known image_update_available state per
	// discovered_container UUID (NORA's internal ID, not the Docker container ID).
	// Used to emit events only on false→true transitions.
	lastUpdateAvailable sync.Map // key: string (NORA container UUID) → bool
}

// NewPortainerEnrichmentWorker returns a worker wired to store.
func NewPortainerEnrichmentWorker(store *repo.Store) *PortainerEnrichmentWorker {
	return &PortainerEnrichmentWorker{store: store}
}

// Start runs the enrichment worker until ctx is cancelled.
// It polls immediately on start, then every 15 minutes.
func (w *PortainerEnrichmentWorker) Start(ctx context.Context) {
	log.Printf("portainer enrichment: starting")

	if err := w.Run(ctx); err != nil && ctx.Err() == nil {
		log.Printf("portainer enrichment: startup run error: %v", err)
	}

	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("portainer enrichment: stopped")
			return
		case <-ticker.C:
			if err := w.Run(ctx); err != nil && ctx.Err() == nil {
				log.Printf("portainer enrichment: run error: %v", err)
			}
		}
	}
}

// Run performs one full enrichment cycle. It is exported so the job registry
// can trigger it on demand.
func (w *PortainerEnrichmentWorker) Run(ctx context.Context) error {
	// ── Step 1: Gate check ────────────────────────────────────────────────────
	components, err := w.store.InfraComponents.List(ctx)
	if err != nil {
		return fmt.Errorf("portainer enrichment: list components: %w", err)
	}

	var portainerComponents []models.InfrastructureComponent
	for _, c := range components {
		if c.Type == "portainer" && c.Enabled {
			portainerComponents = append(portainerComponents, c)
		}
	}
	if len(portainerComponents) == 0 {
		return nil
	}

	// ── Step 2: Load all NORA-known containers once (matched by name) ─────────
	allContainers, err := w.store.DiscoveredContainers.ListAllDiscoveredContainers(ctx)
	if err != nil {
		return fmt.Errorf("portainer enrichment: list discovered containers: %w", err)
	}

	// Build a name→container map for O(1) lookup.
	byName := make(map[string]*models.DiscoveredContainer, len(allContainers))
	for _, c := range allContainers {
		byName[c.ContainerName] = c
	}

	// ── Step 3: Poll each Portainer component ─────────────────────────────────
	totalMatched := 0
	totalEndpoints := 0

	for _, comp := range portainerComponents {
		matched, endpoints, err := w.enrichComponent(ctx, comp, byName)
		if err != nil {
			log.Printf("portainer enrichment: component %q (%s): %v", comp.Name, comp.ID, err)
			_ = w.store.InfraComponents.UpdateStatus(ctx, comp.ID, "offline", time.Now().UTC().Format(time.RFC3339Nano))
			continue
		}
		totalMatched += matched
		totalEndpoints += endpoints
		_ = w.store.InfraComponents.UpdateStatus(ctx, comp.ID, "online", time.Now().UTC().Format(time.RFC3339Nano))
	}

	log.Printf("portainer enrichment: %d containers matched across %d endpoints", totalMatched, totalEndpoints)
	return nil
}

// enrichComponent runs one poll cycle for a single Portainer component.
// Returns (matchCount, endpointCount, error).
func (w *PortainerEnrichmentWorker) enrichComponent(
	ctx context.Context,
	comp models.InfrastructureComponent,
	byName map[string]*models.DiscoveredContainer,
) (int, int, error) {
	if comp.Credentials == nil || *comp.Credentials == "" {
		return 0, 0, fmt.Errorf("no credentials configured")
	}

	creds, err := ParsePortainerCredentials(*comp.Credentials)
	if err != nil {
		return 0, 0, err
	}

	client := NewPortainerClient(creds.BaseURL, creds.APIKey)

	endpoints, err := client.ListEndpoints(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("list endpoints: %w", err)
	}

	matched := 0
	for _, ep := range endpoints {
		containers, err := client.ListContainers(ctx, ep.ID)
		if err != nil {
			log.Printf("portainer enrichment: list containers endpoint %q (%d): %v", ep.Name, ep.ID, err)
			continue
		}

		for _, pc := range containers {
			name := pc.FirstName()
			if name == "" {
				continue
			}

			nora, ok := byName[name]
			if !ok {
				log.Printf("portainer enrichment: no match for container %q endpoint %q (debug)", name, ep.Name)
				continue
			}

			// Determine image_update_available using Portainer's Docker gateway.
			// We compare the manifest digest of the locally running image (from
			// GET /api/endpoints/{id}/docker/images/{imageId}/json → RepoDigests)
			// against the registry_digest stored by DD-9's image poller.
			// If the running manifest digest differs from the latest registry digest
			// and both are non-empty, the image is flagged as having an update available.
			// Portainer's result overwrites DD-9's for all matched containers.
			updateAvailable := w.determineImageUpdate(ctx, client, ep.ID, pc, nora)

			// Persist: overwrite whatever DD-9 last stored.
			imageDigest := pc.ID // use container ID as a stable key when inspect fails
			if err := w.store.DiscoveredContainers.UpdateContainerImageCheck(
				ctx, nora.ID, imageDigest, "", updateAvailable,
			); err != nil {
				log.Printf("portainer enrichment: update image check %s: %v", name, err)
				continue
			}

			// Emit event on false→true transition only.
			if updateAvailable {
				prev, _ := w.lastUpdateAvailable.Load(nora.ID)
				prevBool, _ := prev.(bool)
				if !prevBool {
					w.emitImageUpdateEvent(ctx, comp, name, pc.Image)
				}
			}
			w.lastUpdateAvailable.Store(nora.ID, updateAvailable)

			matched++
		}
	}

	return matched, len(endpoints), nil
}

// determineImageUpdate inspects the running container's image via the Portainer
// Docker gateway and returns true when a newer image is available.
//
// Detection method:
//  1. Call GET /api/endpoints/{id}/docker/containers/{id}/json to get the
//     running image's config hash (Image field, always sha256:...).
//  2. Call GET /api/endpoints/{id}/docker/images/{imageId}/json to retrieve
//     the locally stored RepoDigests (manifest digests in "img@sha256:..." form).
//  3. Extract the manifest digest matching the container's image name:tag.
//  4. Compare against the registry_digest stored by DD-9's image poller.
//     If they differ and both are non-empty → update available.
//
// If the Portainer inspect fails or RepoDigests is empty, falls back to the
// existing image_update_available value from NORA's DB (no change).
func (w *PortainerEnrichmentWorker) determineImageUpdate(
	ctx context.Context,
	client *PortainerClient,
	endpointID int,
	pc PortainerContainer,
	nora *models.DiscoveredContainer,
) bool {
	// Inspect the container to get the running image ID.
	inspect, err := client.InspectContainer(ctx, endpointID, pc.ID)
	if err != nil {
		log.Printf("portainer enrichment: inspect container %s: %v (debug)", pc.FirstName(), err)
		return nora.ImageUpdateAvailable != 0
	}

	// Inspect the image to get manifest digests stored locally.
	imgDetail, err := client.InspectImage(ctx, endpointID, inspect.Image)
	if err != nil {
		log.Printf("portainer enrichment: inspect image %s: %v (debug)", inspect.Image, err)
		return nora.ImageUpdateAvailable != 0
	}

	// Extract the manifest digest for this image's name:tag.
	imageName := inspect.Config.Image
	if imageName == "" {
		imageName = pc.Image
	}
	runningManifest := ExtractManifestDigest(imgDetail.RepoDigests, imageName)

	// If we couldn't get a manifest digest locally, fall back to stored value.
	if runningManifest == "" {
		return nora.ImageUpdateAvailable != 0
	}

	// If DD-9 hasn't checked the registry yet, we can't determine update status.
	if nora.RegistryDigest == nil || *nora.RegistryDigest == "" {
		return nora.ImageUpdateAvailable != 0
	}

	// Update available when locally running manifest ≠ latest registry manifest.
	return runningManifest != *nora.RegistryDigest
}

// emitImageUpdateEvent fires an "image update available" event through the
// unified event store (which is wrapped by the rules engine in main.go so all
// notification rules are evaluated automatically).
func (w *PortainerEnrichmentWorker) emitImageUpdateEvent(
	ctx context.Context,
	comp models.InfrastructureComponent,
	containerName string,
	image string,
) {
	payload, _ := json.Marshal(map[string]string{
		"source":         "portainer_enrichment",
		"container_name": containerName,
		"image":          image,
		"component_id":   comp.ID,
	})
	event := &models.Event{
		ID:         uuid.NewString(),
		Level:      "info",
		SourceName: comp.Name,
		SourceType: "docker_engine",
		SourceID:   comp.ID,
		Title:      fmt.Sprintf("Image update available — %s (%s)", containerName, image),
		Payload:    string(payload),
		CreatedAt:  time.Now().UTC(),
	}
	if err := w.store.Events.Create(ctx, event); err != nil {
		log.Printf("portainer enrichment: emit image update event for %s: %v", containerName, err)
	}
}
