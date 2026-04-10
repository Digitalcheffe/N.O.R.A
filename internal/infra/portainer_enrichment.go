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
// every 15 minutes. For each Portainer endpoint, it lists containers, upserts them
// into discovered_containers (keyed by the Portainer component ID), stores the
// locally running image manifest digest, and emits an event when the registry
// poller later marks image_update_available as true.
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

	// ── Step 2: Poll each Portainer component ────────────────────────────────
	totalUpserted := 0
	totalEndpoints := 0

	for _, comp := range portainerComponents {
		upserted, endpoints, err := w.enrichComponent(ctx, comp)
		if err != nil {
			log.Printf("portainer enrichment: component %q (%s): %v", comp.Name, comp.ID, err)
			_ = w.store.InfraComponents.UpdateStatus(ctx, comp.ID, "offline", time.Now().UTC().Format(time.RFC3339Nano))
			continue
		}
		totalUpserted += upserted
		totalEndpoints += endpoints
		_ = w.store.InfraComponents.UpdateStatus(ctx, comp.ID, "online", time.Now().UTC().Format(time.RFC3339Nano))
	}

	log.Printf("portainer enrichment: %d containers upserted across %d endpoints", totalUpserted, totalEndpoints)
	return nil
}

// enrichComponent runs one poll cycle for a single Portainer component.
// It upserts every container seen across all endpoints into discovered_containers
// (keyed by infra_component_id=comp.ID, container_id=pc.ID), then checks image
// update availability and marks containers that disappeared as stopped.
// Returns (upsertCount, endpointCount, error).
func (w *PortainerEnrichmentWorker) enrichComponent(
	ctx context.Context,
	comp models.InfrastructureComponent,
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

	upserted := 0
	var seenContainerIDs []string // for MarkStoppedIfNotRunning

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

			now := time.Now().UTC()

			// Upsert this container under the Portainer component.
			// source_type="portainer" records provenance; the parent relationship
			// (portainer → container) is written to component_links below.
			// The unique key is (infra_component_id, container_name) so rebuilding
			// a container refreshes container_id in place.  UpsertDiscoveredContainer
			// populates rec.ID with the stable NORA UUID via RETURNING id.
			encJSON := func(v any) *string {
				b, err := json.Marshal(v)
				if err != nil || string(b) == "null" {
					return nil
				}
				s := string(b)
				return &s
			}

			type networkEntry struct {
				Name string `json:"name"`
				IP   string `json:"ip,omitempty"`
			}
			var networks []networkEntry
			if pc.NetworkSettings != nil {
				for netName, ep := range pc.NetworkSettings.Networks {
					entry := networkEntry{Name: netName}
					if ep != nil && ep.IPAddress != "" {
						entry.IP = ep.IPAddress
					}
					networks = append(networks, entry)
				}
			}

			rec := &models.DiscoveredContainer{
				InfraComponentID: comp.ID,
				SourceType:       "portainer",
				ContainerID:      pc.ID,
				ContainerName:    name,
				Image:            pc.Image,
				Status:           pc.State,
				LastSeenAt:       now,
				CreatedAt:        now,
				Ports:            encJSON(pc.Ports),
				Networks:         encJSON(networks),
				Labels:           encJSON(pc.Labels),
			}
			if err := w.store.DiscoveredContainers.UpsertDiscoveredContainer(ctx, rec); err != nil {
				log.Printf("portainer enrichment: upsert container %q: %v", name, err)
				continue
			}

			// Register the parent relationship in component_links.
			if err := w.store.ComponentLinks.SetParent(ctx, "portainer", comp.ID, "container", rec.ID); err != nil {
				log.Printf("portainer enrichment: set parent link for container %q: %v", name, err)
			}

			seenContainerIDs = append(seenContainerIDs, pc.ID)

			// Inspect the container to capture env vars and the local image digest.
			// Both are written in a single inspect call to avoid extra round-trips.
			if localDigest := w.extractInspectData(ctx, client, ep.ID, pc, rec.ID); localDigest != "" {
				if err := w.store.DiscoveredContainers.UpdateContainerLocalDigest(ctx, rec.ID, localDigest); err != nil {
					log.Printf("portainer enrichment: update local digest %q: %v", name, err)
				}
			}

			// Re-fetch the full row so we can read image_update_available as set by
			// the registry poller (the upsert only returns the id).
			if full, fetchErr := w.store.DiscoveredContainers.GetDiscoveredContainer(ctx, rec.ID); fetchErr == nil {
				rec = full
			}

			// Emit event if registry poller already flagged an update (false→true).
			updateAvailable := rec.ImageUpdateAvailable != 0
			if updateAvailable {
				prev, _ := w.lastUpdateAvailable.Load(rec.ID)
				prevBool, _ := prev.(bool)
				if !prevBool {
					w.emitImageUpdateEvent(ctx, comp, name, pc.Image)
				}
			}
			w.lastUpdateAvailable.Store(rec.ID, updateAvailable)

			upserted++
		}
	}

	// Mark containers that no longer appear in any endpoint as stopped.
	if err := w.store.DiscoveredContainers.MarkStoppedIfNotRunning(ctx, comp.ID, seenContainerIDs); err != nil {
		log.Printf("portainer enrichment: mark stopped %s: %v", comp.ID, err)
	}

	return upserted, len(endpoints), nil
}

// extractInspectData calls InspectContainer once to capture env vars and the
// local image manifest digest. Env vars are persisted immediately; the digest
// is returned so the caller can write it via UpdateContainerLocalDigest.
// Returns "" if the inspect fails or no manifest digest is available.
func (w *PortainerEnrichmentWorker) extractInspectData(
	ctx context.Context,
	client *PortainerClient,
	endpointID int,
	pc PortainerContainer,
	containerID string, // NORA UUID — used for the env vars DB write
) string {
	inspect, err := client.InspectContainer(ctx, endpointID, pc.ID)
	if err != nil {
		log.Printf("portainer enrichment: inspect container %s: %v (debug)", pc.FirstName(), err)
		return ""
	}

	// Persist environment variables.
	if len(inspect.Config.Env) > 0 {
		if b, jsonErr := json.Marshal(inspect.Config.Env); jsonErr == nil {
			s := string(b)
			if err := w.store.DiscoveredContainers.UpdateContainerEnvVars(ctx, containerID, s); err != nil {
				log.Printf("portainer enrichment: update env vars %q: %v", pc.FirstName(), err)
			}
		}
	}

	imgDetail, err := client.InspectImage(ctx, endpointID, inspect.Image)
	if err != nil {
		log.Printf("portainer enrichment: inspect image %s: %v (debug)", inspect.Image, err)
		return ""
	}
	imageName := inspect.Config.Image
	if imageName == "" {
		imageName = pc.Image
	}
	return ExtractManifestDigest(imgDetail.RepoDigests, imageName)
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
