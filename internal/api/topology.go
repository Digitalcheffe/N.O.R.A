package api

import (
	"net/http"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

// TopologyHandler serves the infrastructure topology chain view.
// Docker engine CRUD has been retired; docker engines are now managed via
// InfraComponentHandler (/api/v1/infrastructure) with type='docker_engine'.
type TopologyHandler struct {
	infraComponents repo.InfraComponentRepo
	apps            repo.AppRepo
	links           repo.ComponentLinkRepo
}

// NewTopologyHandler creates a TopologyHandler.
func NewTopologyHandler(
	infraComponents repo.InfraComponentRepo,
	apps repo.AppRepo,
	links repo.ComponentLinkRepo,
) *TopologyHandler {
	return &TopologyHandler{
		infraComponents: infraComponents,
		apps:            apps,
		links:           links,
	}
}

// Routes registers topology endpoints on r.
func (h *TopologyHandler) Routes(r chi.Router) {
	r.Get("/topology", h.GetTopology)
}

// ---- response types ---------------------------------------------------------

type topologyApp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type topologyComponent struct {
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	Type     string              `json:"type"`
	Children []topologyComponent `json:"children"`
	Apps     []topologyApp       `json:"apps"`
}

// ---- GET /topology ----------------------------------------------------------

// GetTopology returns the infrastructure component tree with nested apps,
// built from component_links relationships. All component types (including
// docker_engine) appear as nodes in the children array.
// GET /api/v1/topology
func (h *TopologyHandler) GetTopology(w http.ResponseWriter, r *http.Request) {
	allComponents, err := h.infraComponents.List(r.Context())
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
	compByID := make(map[string]models.InfrastructureComponent, len(allComponents))
	for _, c := range allComponents {
		compByID[c.ID] = c
	}

	// Pass 1: apps grouped by parent component ID.
	appsByParent := make(map[string][]topologyApp)
	for _, link := range allLinks {
		if link.ChildType == "app" {
			if a, ok := appByID[link.ChildID]; ok {
				appsByParent[link.ParentID] = append(appsByParent[link.ParentID], topologyApp{ID: a.ID, Name: a.Name})
			}
		}
	}

	// Pass 2: infra component children grouped by parent ID.
	childrenByParent := make(map[string][]models.InfrastructureComponent)
	for _, link := range allLinks {
		if link.ChildType != "app" {
			if c, ok := compByID[link.ChildID]; ok {
				childrenByParent[link.ParentID] = append(childrenByParent[link.ParentID], c)
			}
		}
	}

	// Roots = components not appearing as a non-app child.
	childIDs := make(map[string]bool)
	for _, link := range allLinks {
		if link.ChildType != "app" {
			childIDs[link.ChildID] = true
		}
	}

	var buildNode func(c models.InfrastructureComponent) topologyComponent
	buildNode = func(c models.InfrastructureComponent) topologyComponent {
		apps := appsByParent[c.ID]
		if apps == nil {
			apps = []topologyApp{}
		}
		childNodes := []topologyComponent{}
		for _, child := range childrenByParent[c.ID] {
			childNodes = append(childNodes, buildNode(child))
		}
		return topologyComponent{
			ID:       c.ID,
			Name:     c.Name,
			Type:     c.Type,
			Children: childNodes,
			Apps:     apps,
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
