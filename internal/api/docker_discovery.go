package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

// DockerDiscoveryHandler serves the container discovery endpoints.
type DockerDiscoveryHandler struct {
	store *repo.Store
}

// NewDockerDiscoveryHandler returns a DockerDiscoveryHandler wired to store.
func NewDockerDiscoveryHandler(store *repo.Store) *DockerDiscoveryHandler {
	return &DockerDiscoveryHandler{store: store}
}

// Routes registers the docker discovery endpoints on r.
func (h *DockerDiscoveryHandler) Routes(r chi.Router) {
	r.Get("/docker-engines/{id}/containers", h.ListContainers)
	r.Get("/infrastructure/{id}/routes", h.ListRoutes)
	r.Get("/discovery/all", h.ListAll)
}

// discoveredContainerResponse is the per-container shape returned by the API.
type discoveredContainerResponse struct {
	ID                   string    `json:"id"`
	ContainerName        string    `json:"container_name"`
	Image                string    `json:"image"`
	Status               string    `json:"status"`
	AppID                *string   `json:"app_id"`
	ProfileSuggestion    *string   `json:"profile_suggestion"`
	SuggestionConfidence *int      `json:"suggestion_confidence"`
	CPUPercent           *float64  `json:"cpu_percent"`
	MemPercent           *float64  `json:"mem_percent"`
	LastSeenAt           time.Time `json:"last_seen_at"`
}

type listDiscoveredContainersResponse struct {
	Data  []discoveredContainerResponse `json:"data"`
	Total int                           `json:"total"`
}

// ListContainers returns all discovered containers for a docker engine.
// GET /api/v1/docker-engines/{id}/containers
func (h *DockerDiscoveryHandler) ListContainers(w http.ResponseWriter, r *http.Request) {
	engineID := chi.URLParam(r, "id")

	// Verify the engine exists.
	if _, err := h.store.DockerEngines.Get(r.Context(), engineID); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "docker engine not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	containers, err := h.store.DiscoveredContainers.ListDiscoveredContainers(r.Context(), engineID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Collect container IDs for a batched resource reading lookup.
	containerIDs := make([]string, len(containers))
	for i, c := range containers {
		containerIDs[i] = c.ContainerID
	}

	metrics, err := h.store.Resources.LatestMetrics(r.Context(), "docker_container", containerIDs)
	if err != nil {
		// Non-fatal — return containers without metrics rather than erroring.
		metrics = map[string]map[string]float64{}
	}

	out := make([]discoveredContainerResponse, len(containers))
	for i, c := range containers {
		item := discoveredContainerResponse{
			ID:                   c.ID,
			ContainerName:        c.ContainerName,
			Image:                c.Image,
			Status:               c.Status,
			AppID:                c.AppID,
			ProfileSuggestion:    c.ProfileSuggestion,
			SuggestionConfidence: c.SuggestionConfidence,
			LastSeenAt:           c.LastSeenAt,
		}

		if m, ok := metrics[c.ContainerID]; ok {
			if v, ok := m["cpu_percent"]; ok {
				item.CPUPercent = &v
			}
			if v, ok := m["mem_percent"]; ok {
				item.MemPercent = &v
			}
		}

		out[i] = item
	}

	writeJSON(w, http.StatusOK, listDiscoveredContainersResponse{Data: out, Total: len(out)})
}

// ── Routes ────────────────────────────────────────────────────────────────────

// discoveredRouteResponse is the per-route shape returned by the routes API.
type discoveredRouteResponse struct {
	ID             string     `json:"id"`
	RouterName     string     `json:"router_name"`
	Domain         *string    `json:"domain"`
	BackendService *string    `json:"backend_service"`
	ContainerID    *string    `json:"container_id"`
	ContainerName  *string    `json:"container_name"`
	AppID          *string    `json:"app_id"`
	SSLExpiry      *time.Time `json:"ssl_expiry"`
	SSLIssuer      *string    `json:"ssl_issuer"`
	LastSeenAt     time.Time  `json:"last_seen_at"`
}

type listDiscoveredRoutesResponse struct {
	Data  []discoveredRouteResponse `json:"data"`
	Total int                       `json:"total"`
}

// buildRouteResponses converts a slice of DiscoveredRoute models to API
// response structs, denormalising container_name from containersByID.
func buildRouteResponses(routes []*models.DiscoveredRoute, containersByID map[string]string) []discoveredRouteResponse {
	out := make([]discoveredRouteResponse, len(routes))
	for i, ro := range routes {
		item := discoveredRouteResponse{
			ID:             ro.ID,
			RouterName:     ro.RouterName,
			Domain:         ro.Domain,
			BackendService: ro.BackendService,
			ContainerID:    ro.ContainerID,
			AppID:          ro.AppID,
			SSLExpiry:      ro.SSLExpiry,
			SSLIssuer:      ro.SSLIssuer,
			LastSeenAt:     ro.LastSeenAt,
		}
		if ro.ContainerID != nil {
			if name, ok := containersByID[*ro.ContainerID]; ok {
				item.ContainerName = &name
			}
		}
		out[i] = item
	}
	return out
}

// containerNameIndex builds a map of discovered_container UUID → container_name
// from a slice of all containers.
func containerNameIndex(containers []*models.DiscoveredContainer) map[string]string {
	m := make(map[string]string, len(containers))
	for _, c := range containers {
		m[c.ID] = c.ContainerName
	}
	return m
}

// ListRoutes returns all discovered routes for a Traefik infrastructure component.
// GET /api/v1/infrastructure/{id}/routes
func (h *DockerDiscoveryHandler) ListRoutes(w http.ResponseWriter, r *http.Request) {
	componentID := chi.URLParam(r, "id")

	if _, err := h.store.InfraComponents.Get(r.Context(), componentID); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "infrastructure component not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	routes, err := h.store.DiscoveredRoutes.ListDiscoveredRoutes(r.Context(), componentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	allContainers, err := h.store.DiscoveredContainers.ListAllDiscoveredContainers(r.Context())
	if err != nil {
		allContainers = nil
	}

	out := buildRouteResponses(routes, containerNameIndex(allContainers))
	writeJSON(w, http.StatusOK, listDiscoveredRoutesResponse{Data: out, Total: len(out)})
}

// ── All ───────────────────────────────────────────────────────────────────────

type discoveryAllResponse struct {
	Containers []discoveredContainerResponse `json:"containers"`
	Routes     []discoveredRouteResponse     `json:"routes"`
}

// ListAll returns all discovered containers and routes across every component.
// GET /api/v1/discovery/all
func (h *DockerDiscoveryHandler) ListAll(w http.ResponseWriter, r *http.Request) {
	allContainers, err := h.store.DiscoveredContainers.ListAllDiscoveredContainers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Collect container IDs for batched metrics lookup.
	containerIDs := make([]string, len(allContainers))
	for i, c := range allContainers {
		containerIDs[i] = c.ContainerID
	}
	metrics, err := h.store.Resources.LatestMetrics(r.Context(), "docker_container", containerIDs)
	if err != nil {
		metrics = map[string]map[string]float64{}
	}

	containerOut := make([]discoveredContainerResponse, len(allContainers))
	for i, c := range allContainers {
		item := discoveredContainerResponse{
			ID:                   c.ID,
			ContainerName:        c.ContainerName,
			Image:                c.Image,
			Status:               c.Status,
			AppID:                c.AppID,
			ProfileSuggestion:    c.ProfileSuggestion,
			SuggestionConfidence: c.SuggestionConfidence,
			LastSeenAt:           c.LastSeenAt,
		}
		if m, ok := metrics[c.ContainerID]; ok {
			if v, ok := m["cpu_percent"]; ok {
				item.CPUPercent = &v
			}
			if v, ok := m["mem_percent"]; ok {
				item.MemPercent = &v
			}
		}
		containerOut[i] = item
	}

	allRoutes, err := h.store.DiscoveredRoutes.ListAllDiscoveredRoutes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	routeOut := buildRouteResponses(allRoutes, containerNameIndex(allContainers))
	writeJSON(w, http.StatusOK, discoveryAllResponse{
		Containers: containerOut,
		Routes:     routeOut,
	})
}
