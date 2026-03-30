package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/docker"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// DockerDiscoveryHandler serves the container discovery endpoints.
type DockerDiscoveryHandler struct {
	store    *repo.Store
	profiles apptemplate.Loader
}

// NewDockerDiscoveryHandler returns a DockerDiscoveryHandler wired to store and profiles.
func NewDockerDiscoveryHandler(store *repo.Store, profiles apptemplate.Loader) *DockerDiscoveryHandler {
	return &DockerDiscoveryHandler{store: store, profiles: profiles}
}

// Routes registers the docker discovery endpoints on r.
func (h *DockerDiscoveryHandler) Routes(r chi.Router) {
	r.Get("/infrastructure/{id}/containers", h.ListContainers)
	r.Get("/infrastructure/{id}/routes", h.ListRoutes)
	r.Get("/discovery/all", h.ListAll)
	r.Delete("/discovered-containers/{id}", h.DeleteContainer)
	r.Post("/discovered-containers/{id}/link-app", h.LinkContainerApp)
	r.Delete("/discovered-containers/{id}/link-app", h.UnlinkContainerApp)
	r.Post("/discovered-routes/{id}/link-app", h.LinkRouteApp)
	r.Delete("/discovered-routes/{id}/link-app", h.UnlinkRouteApp)
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

// ListContainers returns all discovered containers for a docker_engine infrastructure component.
// GET /api/v1/infrastructure/{id}/containers
func (h *DockerDiscoveryHandler) ListContainers(w http.ResponseWriter, r *http.Request) {
	componentID := chi.URLParam(r, "id")

	// Verify the infrastructure component exists.
	if _, err := h.store.InfraComponents.Get(r.Context(), componentID); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "infrastructure component not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	containers, err := h.store.DiscoveredContainers.ListDiscoveredContainers(r.Context(), componentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Collect all IDs needed for the resource lookup.
	// Priority order for metric lookup per container:
	//   1. StableContainerSourceID(componentID, name) — used by ResourcePoller since stable-ID change
	//   2. appID — used when container is linked to an app (ResourcePoller uses appID in that case)
	//   3. raw containerID — backward compat for readings recorded before the stable-ID change
	lookupIDs := make([]string, 0, len(containers)*3)
	for _, c := range containers {
		lookupIDs = append(lookupIDs, docker.StableContainerSourceID(componentID, c.ContainerName))
		if c.AppID != nil && *c.AppID != "" {
			lookupIDs = append(lookupIDs, *c.AppID)
		}
		lookupIDs = append(lookupIDs, c.ContainerID)
	}

	metrics, err := h.store.Resources.LatestMetrics(r.Context(), "docker_container", lookupIDs)
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

		// Walk lookup priority until we find a source ID that has metrics.
		candidates := []string{docker.StableContainerSourceID(componentID, c.ContainerName)}
		if c.AppID != nil && *c.AppID != "" {
			candidates = append(candidates, *c.AppID)
		}
		candidates = append(candidates, c.ContainerID)

		for _, sourceID := range candidates {
			if m, ok := metrics[sourceID]; ok {
				if v, ok := m["cpu_percent"]; ok {
					item.CPUPercent = &v
				}
				if v, ok := m["mem_percent"]; ok {
					item.MemPercent = &v
				}
				break
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

// ── Link / Unlink ─────────────────────────────────────────────────────────────

type linkAppRequest struct {
	Mode      string          `json:"mode"`       // "existing" | "create"
	AppID     string          `json:"app_id"`     // mode=existing
	ProfileID string          `json:"profile_id"` // mode=create
	Name      string          `json:"name"`       // mode=create
	Config    json.RawMessage `json:"config"`     // mode=create
}

// LinkContainerApp links a discovered container to an app.
// POST /api/v1/discovered-containers/{id}/link-app
func (h *DockerDiscoveryHandler) LinkContainerApp(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	container, err := h.store.DiscoveredContainers.GetDiscoveredContainer(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "discovered container not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var req linkAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	switch req.Mode {
	case "existing":
		if req.AppID == "" {
			writeError(w, http.StatusUnprocessableEntity, "app_id is required for mode=existing")
			return
		}
		if _, err := h.store.Apps.Get(r.Context(), req.AppID); errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusUnprocessableEntity, "app_id does not exist")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := h.store.DiscoveredContainers.SetDiscoveredContainerApp(r.Context(), id, req.AppID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		container.AppID = &req.AppID
		_ = docker.EnrichAppOnLink(r.Context(), h.store, h.profiles, req.AppID, &id, nil)
		writeJSON(w, http.StatusOK, container)

	case "create":
		if req.Name == "" {
			writeError(w, http.StatusUnprocessableEntity, "name is required for mode=create")
			return
		}
		if req.ProfileID == "" {
			writeError(w, http.StatusUnprocessableEntity, "profile_id is required for mode=create")
			return
		}
		if t, _ := h.profiles.Get(req.ProfileID); t == nil {
			writeError(w, http.StatusUnprocessableEntity, "profile_id does not exist")
			return
		}
		token, err := generateToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate token")
			return
		}
		cfg := models.ConfigJSON("{}")
		if len(req.Config) > 0 {
			cfg = models.ConfigJSON(req.Config)
		}
		app := &models.App{
			ID:        uuid.New().String(),
			Name:      req.Name,
			Token:     token,
			ProfileID: req.ProfileID,
			Config:    cfg,
			RateLimit: 100,
			CreatedAt: time.Now().UTC(),
		}
		if err := h.store.Apps.Create(r.Context(), app); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := h.store.DiscoveredContainers.SetDiscoveredContainerApp(r.Context(), id, app.ID); err != nil {
			// Best-effort rollback — orphaned app is better than silent failure.
			_ = h.store.Apps.Delete(r.Context(), app.ID)
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = docker.EnrichAppOnLink(r.Context(), h.store, h.profiles, app.ID, &id, nil)
		writeJSON(w, http.StatusCreated, app)

	default:
		writeError(w, http.StatusUnprocessableEntity, "mode must be 'existing' or 'create'")
	}
}

// DeleteContainer hard-deletes a discovered container record.
// DELETE /api/v1/discovered-containers/{id}
func (h *DockerDiscoveryHandler) DeleteContainer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.store.DiscoveredContainers.GetDiscoveredContainer(r.Context(), id); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "discovered container not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.store.DiscoveredContainers.DeleteDiscoveredContainer(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UnlinkContainerApp sets app_id back to null on a discovered container.
// DELETE /api/v1/discovered-containers/{id}/link-app
func (h *DockerDiscoveryHandler) UnlinkContainerApp(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.store.DiscoveredContainers.GetDiscoveredContainer(r.Context(), id); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "discovered container not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.store.DiscoveredContainers.ClearDiscoveredContainerApp(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LinkRouteApp links a discovered route to an app, and auto-creates an SSL check
// for the route's domain if one does not already exist.
// POST /api/v1/discovered-routes/{id}/link-app
func (h *DockerDiscoveryHandler) LinkRouteApp(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	route, err := h.store.DiscoveredRoutes.GetDiscoveredRoute(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "discovered route not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var req linkAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var linkedAppID string

	switch req.Mode {
	case "existing":
		if req.AppID == "" {
			writeError(w, http.StatusUnprocessableEntity, "app_id is required for mode=existing")
			return
		}
		if _, err := h.store.Apps.Get(r.Context(), req.AppID); errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusUnprocessableEntity, "app_id does not exist")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := h.store.DiscoveredRoutes.SetDiscoveredRouteApp(r.Context(), id, req.AppID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		linkedAppID = req.AppID
		route.AppID = &linkedAppID
		_ = docker.EnrichAppOnLink(r.Context(), h.store, h.profiles, linkedAppID, nil, &id)
		writeJSON(w, http.StatusOK, route)


	case "create":
		if req.Name == "" {
			writeError(w, http.StatusUnprocessableEntity, "name is required for mode=create")
			return
		}
		if req.ProfileID == "" {
			writeError(w, http.StatusUnprocessableEntity, "profile_id is required for mode=create")
			return
		}
		if t, _ := h.profiles.Get(req.ProfileID); t == nil {
			writeError(w, http.StatusUnprocessableEntity, "profile_id does not exist")
			return
		}
		token, err := generateToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate token")
			return
		}
		cfg := models.ConfigJSON("{}")
		if len(req.Config) > 0 {
			cfg = models.ConfigJSON(req.Config)
		}
		app := &models.App{
			ID:        uuid.New().String(),
			Name:      req.Name,
			Token:     token,
			ProfileID: req.ProfileID,
			Config:    cfg,
			RateLimit: 100,
			CreatedAt: time.Now().UTC(),
		}
		if err := h.store.Apps.Create(r.Context(), app); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := h.store.DiscoveredRoutes.SetDiscoveredRouteApp(r.Context(), id, app.ID); err != nil {
			_ = h.store.Apps.Delete(r.Context(), app.ID)
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		linkedAppID = app.ID
		route.AppID = &linkedAppID
		_ = docker.EnrichAppOnLink(r.Context(), h.store, h.profiles, linkedAppID, nil, &id)
		writeJSON(w, http.StatusCreated, app)

	default:
		writeError(w, http.StatusUnprocessableEntity, "mode must be 'existing' or 'create'")
	}
}

// UnlinkRouteApp sets app_id back to null on a discovered route.
// DELETE /api/v1/discovered-routes/{id}/link-app
func (h *DockerDiscoveryHandler) UnlinkRouteApp(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.store.DiscoveredRoutes.GetDiscoveredRoute(r.Context(), id); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "discovered route not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.store.DiscoveredRoutes.ClearDiscoveredRouteApp(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

