package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

// PortainerHandler serves the Portainer-specific API endpoints.
// All endpoints proxy to the configured Portainer instance via its REST API.
type PortainerHandler struct {
	store *repo.Store
}

// NewPortainerHandler returns a handler wired to store.
func NewPortainerHandler(store *repo.Store) *PortainerHandler {
	return &PortainerHandler{store: store}
}

// Routes registers Portainer endpoints on r.
func (h *PortainerHandler) Routes(r chi.Router) {
	r.Get("/integrations/portainer/{componentId}/endpoints", h.ListEndpoints)
	r.Get("/integrations/portainer/{componentId}/endpoints/{endpointId}/summary", h.GetEndpointSummary)
	r.Get("/integrations/portainer/{componentId}/endpoints/{endpointId}/containers", h.GetEndpointContainers)
}

// ── response types ────────────────────────────────────────────────────────────

type portainerEndpointResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Type int    `json:"type"`
}

// portainerEndpointSummary aggregates resource counts for one Portainer endpoint.
type portainerEndpointSummary struct {
	ContainersRunning int   `json:"containers_running"`
	ContainersStopped int   `json:"containers_stopped"`
	ImagesTotal       int   `json:"images_total"`
	ImagesDangling    int   `json:"images_dangling"`
	ImagesDiskBytes   int64 `json:"images_disk_bytes"`
	VolumesTotal      int   `json:"volumes_total"`
	VolumesUnused     int   `json:"volumes_unused"`
	VolumesDiskBytes  int64 `json:"volumes_disk_bytes"`
	NetworksTotal     int   `json:"networks_total"`
}

// portainerContainerResource is the live resource snapshot for one container.
type portainerContainerResource struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	Image                string  `json:"image"`
	State                string  `json:"state"`
	CPUPercent           float64 `json:"cpu_percent"`
	MemBytes             uint64  `json:"mem_bytes"`
	MemLimitBytes        uint64  `json:"mem_limit_bytes"`
	MemPercent           float64 `json:"mem_percent"`
	ImageUpdateAvailable bool    `json:"image_update_available"`
	Stack                string  `json:"stack,omitempty"`
}

// ── handlers ──────────────────────────────────────────────────────────────────

// ListEndpoints returns all Portainer environments for the given component.
// GET /api/v1/integrations/portainer/{componentId}/endpoints
func (h *PortainerHandler) ListEndpoints(w http.ResponseWriter, r *http.Request) {
	client, ok := h.loadClient(w, r)
	if !ok {
		return
	}

	endpoints, err := client.ListEndpoints(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "portainer unreachable: "+err.Error())
		return
	}

	out := make([]portainerEndpointResponse, len(endpoints))
	for i, ep := range endpoints {
		out[i] = portainerEndpointResponse{ID: ep.ID, Name: ep.Name, Type: ep.Type}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": out, "total": len(out)})
}

// GetEndpointSummary returns aggregate resource counts for one Portainer endpoint.
// GET /api/v1/integrations/portainer/{componentId}/endpoints/{endpointId}/summary
func (h *PortainerHandler) GetEndpointSummary(w http.ResponseWriter, r *http.Request) {
	client, ok := h.loadClient(w, r)
	if !ok {
		return
	}

	endpointID, err := strconv.Atoi(chi.URLParam(r, "endpointId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid endpoint id")
		return
	}

	ctx := r.Context()

	// Fetch all four resource types in parallel.
	type result[T any] struct {
		val T
		err error
	}
	contCh := make(chan result[[]infra.PortainerContainer], 1)
	imgCh := make(chan result[[]infra.PortainerImageSummary], 1)
	volCh := make(chan result[*infra.PortainerVolumesResponse], 1)
	netCh := make(chan result[[]infra.PortainerNetwork], 1)

	go func() {
		v, e := client.ListContainers(ctx, endpointID)
		contCh <- result[[]infra.PortainerContainer]{v, e}
	}()
	go func() {
		v, e := client.ListImages(ctx, endpointID)
		imgCh <- result[[]infra.PortainerImageSummary]{v, e}
	}()
	go func() {
		v, e := client.ListVolumes(ctx, endpointID)
		volCh <- result[*infra.PortainerVolumesResponse]{v, e}
	}()
	go func() {
		v, e := client.ListNetworks(ctx, endpointID)
		netCh <- result[[]infra.PortainerNetwork]{v, e}
	}()

	contRes := <-contCh
	imgRes := <-imgCh
	volRes := <-volCh
	netRes := <-netCh

	for _, e := range []error{contRes.err, imgRes.err, volRes.err, netRes.err} {
		if e != nil {
			writeError(w, http.StatusBadGateway, "portainer error: "+e.Error())
			return
		}
	}

	// Compute summary.
	summary := portainerEndpointSummary{}

	// Containers.
	runningIDs := make(map[string]bool)
	for _, c := range contRes.val {
		if c.State == "running" {
			summary.ContainersRunning++
			runningIDs[c.ID] = true
		} else {
			summary.ContainersStopped++
		}
	}

	// Images: dangling = Containers == 0 (no container references this image).
	for _, img := range imgRes.val {
		summary.ImagesTotal++
		summary.ImagesDiskBytes += img.Size
		if img.Containers == 0 {
			summary.ImagesDangling++
		}
	}

	// Volumes: unused = RefCount == 0.
	if volRes.val != nil {
		for _, vol := range volRes.val.Volumes {
			summary.VolumesTotal++
			if vol.UsageData != nil {
				summary.VolumesDiskBytes += vol.UsageData.Size
				if vol.UsageData.RefCount == 0 {
					summary.VolumesUnused++
				}
			} else {
				// No UsageData → assume unused (Docker may not collect it without --volumes flag).
				summary.VolumesUnused++
			}
		}
	}

	// Networks.
	summary.NetworksTotal = len(netRes.val)

	writeJSON(w, http.StatusOK, summary)
}

// GetEndpointContainers returns live CPU/memory stats for all containers in one endpoint.
// Stats are fetched in parallel with a semaphore capping concurrency at 10.
// GET /api/v1/integrations/portainer/{componentId}/endpoints/{endpointId}/containers
func (h *PortainerHandler) GetEndpointContainers(w http.ResponseWriter, r *http.Request) {
	client, ok := h.loadClient(w, r)
	if !ok {
		return
	}

	endpointID, err := strconv.Atoi(chi.URLParam(r, "endpointId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid endpoint id")
		return
	}

	ctx := r.Context()

	containers, err := client.ListContainers(ctx, endpointID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "portainer error: "+err.Error())
		return
	}

	// Load NORA's stored image_update_available for all containers by name.
	storedContainers, _ := h.store.DiscoveredContainers.ListAllDiscoveredContainers(ctx)
	updateByName := make(map[string]bool, len(storedContainers))
	for _, sc := range storedContainers {
		updateByName[sc.ContainerName] = sc.ImageUpdateAvailable != 0
	}

	// Fetch live stats in parallel, semaphore limits to 10 concurrent requests.
	const maxConcurrent = 10
	sem := make(chan struct{}, maxConcurrent)
	var mu sync.Mutex
	var wg sync.WaitGroup

	results := make([]portainerContainerResource, 0, len(containers))

	for _, c := range containers {
		wg.Add(1)
		go func(pc infra.PortainerContainer) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			res := portainerContainerResource{
				ID:    pc.ID,
				Name:  pc.FirstName(),
				Image: pc.Image,
				State: pc.State,
				Stack: pc.StackName(),
			}

			// Check NORA's stored image_update_available.
			if upd, ok := updateByName[res.Name]; ok {
				res.ImageUpdateAvailable = upd
			}

			// Only fetch live stats for running containers.
			if pc.State == "running" {
				// Use a 5-second per-container timeout.
				statsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				if stats, err := client.GetContainerStats(statsCtx, endpointID, pc.ID); err == nil {
					res.CPUPercent = infra.CalcCPUPercent(stats)
					rss := stats.MemoryStats.Usage
					if stats.MemoryStats.Stats.Cache < rss {
						rss -= stats.MemoryStats.Stats.Cache
					}
					res.MemBytes = rss
					res.MemLimitBytes = stats.MemoryStats.Limit
					res.MemPercent = infra.CalcMemPercent(stats)
				}
			}

			mu.Lock()
			results = append(results, res)
			mu.Unlock()
		}(c)
	}
	wg.Wait()

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": results, "total": len(results)})
}

// ── helpers ───────────────────────────────────────────────────────────────────

// loadClient loads the portainer infrastructure component by ID from the URL,
// parses its credentials, and returns a ready-to-use PortainerClient.
// On error it writes an appropriate HTTP error and returns (nil, false).
func (h *PortainerHandler) loadClient(w http.ResponseWriter, r *http.Request) (*infra.PortainerClient, bool) {
	componentID := chi.URLParam(r, "componentId")
	comp, err := h.store.InfraComponents.Get(r.Context(), componentID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusNotFound, "infrastructure component not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return nil, false
	}
	if comp.Type != "portainer" {
		writeError(w, http.StatusBadRequest, "component is not a portainer type")
		return nil, false
	}
	if comp.Credentials == nil || *comp.Credentials == "" {
		writeError(w, http.StatusBadRequest, "portainer component has no credentials configured")
		return nil, false
	}
	creds, err := infra.ParsePortainerCredentials(*comp.Credentials)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "invalid credentials: "+err.Error())
		return nil, false
	}
	return infra.NewPortainerClient(creds.BaseURL, creds.APIKey), true
}
