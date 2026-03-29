package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/digitalcheffe/nora/internal/apptemplate"
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
	r.Get("/docker-engines/{id}/containers", h.ListContainers)
	r.Get("/infrastructure/{id}/routes", h.ListRoutes)
	r.Get("/discovery/all", h.ListAll)
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
		app, err := h.store.Apps.Get(r.Context(), req.AppID)
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusUnprocessableEntity, "app_id does not exist")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := h.store.DiscoveredContainers.SetDiscoveredContainerApp(r.Context(), id, req.AppID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// Set docker_engine_id on the app if not already set.
		if app.DockerEngineID == "" {
			_ = h.store.Apps.SetDockerEngineID(r.Context(), req.AppID, container.DockerEngineID)
		}
		container.AppID = &req.AppID
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
			ID:             uuid.New().String(),
			Name:           req.Name,
			Token:          token,
			ProfileID:      req.ProfileID,
			DockerEngineID: container.DockerEngineID,
			Config:         cfg,
			RateLimit:      100,
			CreatedAt:      time.Now().UTC(),
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
		writeJSON(w, http.StatusCreated, app)

	default:
		writeError(w, http.StatusUnprocessableEntity, "mode must be 'existing' or 'create'")
	}
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
		h.maybeCreateSSLCheck(r.Context(), route)
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
		h.maybeCreateSSLCheck(r.Context(), route)
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

// maybeCreateSSLCheck creates an ssl monitor check for route.Domain if the route
// has a domain and no ssl check for that domain already exists.
func (h *DockerDiscoveryHandler) maybeCreateSSLCheck(ctx context.Context, route *models.DiscoveredRoute) {
	if route.Domain == nil || *route.Domain == "" {
		return
	}
	domain := *route.Domain
	exists, err := h.store.Checks.ExistsForTypeAndTarget(ctx, "ssl", domain)
	if err != nil || exists {
		return
	}
	check := &models.MonitorCheck{
		ID:           uuid.New().String(),
		Name:         "SSL — " + domain,
		Type:         "ssl",
		Target:       domain,
		IntervalSecs: 3600,
		SSLWarnDays:  30,
		SSLCritDays:  7,
		Enabled:      true,
		CreatedAt:    time.Now().UTC(),
	}
	_ = h.store.Checks.Create(ctx, check)
}
