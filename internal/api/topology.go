package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// TopologyHandler holds dependencies for docker engine endpoints and the
// topology chain view. Physical/virtual host management has moved to
// InfraComponentHandler (/api/v1/infrastructure).
type TopologyHandler struct {
	infraComponents repo.InfraComponentRepo
	dockerEngines   repo.DockerEngineRepo
	apps            repo.AppRepo
	rollups         repo.ResourceRollupRepo
	links           repo.ComponentLinkRepo
}

// NewTopologyHandler creates a TopologyHandler.
func NewTopologyHandler(
	infraComponents repo.InfraComponentRepo,
	dockerEngines repo.DockerEngineRepo,
	apps repo.AppRepo,
	rollups repo.ResourceRollupRepo,
	links repo.ComponentLinkRepo,
) *TopologyHandler {
	return &TopologyHandler{
		infraComponents: infraComponents,
		dockerEngines:   dockerEngines,
		apps:            apps,
		rollups:         rollups,
		links:           links,
	}
}

// Routes registers all topology endpoints on r.
func (h *TopologyHandler) Routes(r chi.Router) {
	// Docker engines
	r.Get("/docker-engines", h.ListDockerEngines)
	r.Post("/docker-engines", h.CreateDockerEngine)
	r.Get("/docker-engines/{id}", h.GetDockerEngine)
	r.Put("/docker-engines/{id}", h.UpdateDockerEngine)
	r.Delete("/docker-engines/{id}", h.DeleteDockerEngine)

	// Full topology chain
	r.Get("/topology", h.GetTopology)
}

// ---- request / response types -----------------------------------------------

type dockerEngineRequest struct {
	Name             string `json:"name"`
	SocketType       string `json:"socket_type"`
	SocketPath       string `json:"socket_path"`
	InfraComponentID string `json:"infra_component_id"`
}

// hostResourcesResponse holds the latest avg rollup values for CPU, memory, and disk.
type hostResourcesResponse struct {
	CPU  float64 `json:"cpu"`
	Mem  float64 `json:"mem"`
	Disk float64 `json:"disk"`
}

// topology chain response types

type topologyApp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type topologyDockerEngine struct {
	ID   string        `json:"id"`
	Name string        `json:"name"`
	Apps []topologyApp `json:"apps"`
}

type topologyComponent struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Type          string                 `json:"type"`
	Children      []topologyComponent    `json:"children"`
	DockerEngines []topologyDockerEngine `json:"docker_engines"`
}

// ---- validation helpers -----------------------------------------------------

var validSocketTypes = map[string]bool{"local": true, "remote_proxy": true}

// ---- docker engine handlers -------------------------------------------------

// ListDockerEngines returns all docker engines.
// GET /api/v1/docker-engines
func (h *TopologyHandler) ListDockerEngines(w http.ResponseWriter, r *http.Request) {
	engines, err := h.dockerEngines.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if engines == nil {
		engines = []models.DockerEngine{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": engines, "total": len(engines)})
}

// CreateDockerEngine creates a docker engine.
// POST /api/v1/docker-engines
func (h *TopologyHandler) CreateDockerEngine(w http.ResponseWriter, r *http.Request) {
	var req dockerEngineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !validSocketTypes[req.SocketType] {
		writeError(w, http.StatusBadRequest, "socket_type must be local or remote_proxy")
		return
	}
	if req.SocketPath == "" {
		writeError(w, http.StatusBadRequest, "socket_path is required")
		return
	}

	// Validate and fetch parent if provided.
	var parent *models.InfrastructureComponent
	if req.InfraComponentID != "" {
		var err error
		parent, err = h.infraComponents.Get(r.Context(), req.InfraComponentID)
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusUnprocessableEntity, "infra_component_id not found")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	engine := &models.DockerEngine{
		ID:         uuid.New().String(),
		Name:       req.Name,
		SocketType: req.SocketType,
		SocketPath: req.SocketPath,
		CreatedAt:  time.Now().UTC(),
	}
	if err := h.dockerEngines.Create(r.Context(), engine); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if parent != nil {
		if err := h.links.SetParent(r.Context(), parent.Type, parent.ID, "docker_engine", engine.ID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusCreated, engine)
}

// GetDockerEngine returns a single docker engine.
// GET /api/v1/docker-engines/{id}
func (h *TopologyHandler) GetDockerEngine(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	engine, err := h.dockerEngines.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "docker engine not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, engine)
}

// UpdateDockerEngine replaces mutable fields on a docker engine.
// PUT /api/v1/docker-engines/{id}
func (h *TopologyHandler) UpdateDockerEngine(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := h.dockerEngines.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "docker engine not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var req dockerEngineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SocketType != "" && !validSocketTypes[req.SocketType] {
		writeError(w, http.StatusBadRequest, "socket_type must be local or remote_proxy")
		return
	}

	// Validate new parent if provided.
	var parent *models.InfrastructureComponent
	if req.InfraComponentID != "" {
		parent, err = h.infraComponents.Get(r.Context(), req.InfraComponentID)
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusUnprocessableEntity, "infra_component_id not found")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.SocketType != "" {
		existing.SocketType = req.SocketType
	}
	if req.SocketPath != "" {
		existing.SocketPath = req.SocketPath
	}

	if err := h.dockerEngines.Update(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Update parent link.
	if parent != nil {
		if err := h.links.SetParent(r.Context(), parent.Type, parent.ID, "docker_engine", existing.ID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else if req.InfraComponentID == "" {
		// Explicitly clear parent (empty string in request = remove link).
		_ = h.links.RemoveParent(r.Context(), "docker_engine", existing.ID)
	}

	writeJSON(w, http.StatusOK, existing)
}

// DeleteDockerEngine removes a docker engine.
// DELETE /api/v1/docker-engines/{id}
func (h *TopologyHandler) DeleteDockerEngine(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.dockerEngines.Delete(r.Context(), id); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "docker engine not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- full topology chain ----------------------------------------------------

// GetTopology returns the infrastructure component tree with nested docker engines
// and apps, built from component_links relationships.
// GET /api/v1/topology
func (h *TopologyHandler) GetTopology(w http.ResponseWriter, r *http.Request) {
	allComponents, err := h.infraComponents.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	engines, err := h.dockerEngines.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	allApps, err := h.apps.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	allLinks, err := h.links.ListAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Build lookup maps.
	appByID := make(map[string]models.App, len(allApps))
	for _, a := range allApps {
		appByID[a.ID] = a
	}
	engineByID := make(map[string]models.DockerEngine, len(engines))
	for _, e := range engines {
		engineByID[e.ID] = e
	}
	compByID := make(map[string]models.InfrastructureComponent, len(allComponents))
	for _, c := range allComponents {
		compByID[c.ID] = c
	}

	// Process links in three passes.

	// Pass 1: apps by parent engine ID.
	appsByEngine := make(map[string][]topologyApp)
	for _, link := range allLinks {
		if link.ChildType == "app" {
			if a, ok := appByID[link.ChildID]; ok {
				appsByEngine[link.ParentID] = append(appsByEngine[link.ParentID], topologyApp{ID: a.ID, Name: a.Name})
			}
		}
	}

	// Pass 2: engines by parent infra component ID.
	enginesByComponent := make(map[string][]topologyDockerEngine)
	for _, link := range allLinks {
		if link.ChildType == "docker_engine" {
			if e, ok := engineByID[link.ChildID]; ok {
				apps := appsByEngine[e.ID]
				if apps == nil {
					apps = []topologyApp{}
				}
				enginesByComponent[link.ParentID] = append(enginesByComponent[link.ParentID], topologyDockerEngine{
					ID:   e.ID,
					Name: e.Name,
					Apps: apps,
				})
			}
		}
	}

	// Pass 3: infra component children by parent ID.
	childrenByParent := make(map[string][]models.InfrastructureComponent)
	for _, link := range allLinks {
		if link.ChildType != "app" && link.ChildType != "docker_engine" {
			if c, ok := compByID[link.ChildID]; ok {
				childrenByParent[link.ParentID] = append(childrenByParent[link.ParentID], c)
			}
		}
	}

	// Build response tree recursively (roots = components with no entry as a child).
	childIDs := make(map[string]bool)
	for _, link := range allLinks {
		if link.ChildType != "app" && link.ChildType != "docker_engine" {
			childIDs[link.ChildID] = true
		}
	}

	var buildNode func(c models.InfrastructureComponent) topologyComponent
	buildNode = func(c models.InfrastructureComponent) topologyComponent {
		des := enginesByComponent[c.ID]
		if des == nil {
			des = []topologyDockerEngine{}
		}
		childNodes := []topologyComponent{}
		for _, child := range childrenByParent[c.ID] {
			childNodes = append(childNodes, buildNode(child))
		}
		return topologyComponent{
			ID:            c.ID,
			Name:          c.Name,
			Type:          c.Type,
			Children:      childNodes,
			DockerEngines: des,
		}
	}

	result := []topologyComponent{}
	for _, c := range allComponents {
		if !childIDs[c.ID] {
			result = append(result, buildNode(c))
		}
	}

	writeJSON(w, http.StatusOK, result)
}
