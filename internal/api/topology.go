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
	containers      repo.DiscoveredContainerRepo
}

// NewTopologyHandler creates a TopologyHandler.
func NewTopologyHandler(
	infraComponents repo.InfraComponentRepo,
	apps repo.AppRepo,
	links repo.ComponentLinkRepo,
	containers repo.DiscoveredContainerRepo,
) *TopologyHandler {
	return &TopologyHandler{
		infraComponents: infraComponents,
		apps:            apps,
		links:           links,
		containers:      containers,
	}
}

// Routes registers topology endpoints on r.
func (h *TopologyHandler) Routes(r chi.Router) {
	r.Get("/topology", h.GetTopology)
}

// ---- response types ---------------------------------------------------------

type topologyApp struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	IconURL string `json:"icon_url,omitempty"`
}

type topologyComponent struct {
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	Type     string              `json:"type"`
	Status   string              `json:"status,omitempty"`
	IP       string              `json:"ip,omitempty"`
	Notes    string              `json:"notes,omitempty"`
	Meta     string              `json:"meta,omitempty"`
	AppID      string              `json:"app_id,omitempty"`
	AppName    string              `json:"app_name,omitempty"`
	AppIconURL string              `json:"app_icon_url,omitempty"`
	Children []topologyComponent `json:"children"`
	Apps     []topologyApp       `json:"apps"`
}

// ---- GET /topology ----------------------------------------------------------

// GetTopology returns the infrastructure component tree with nested apps and
// discovered containers, built from component_links relationships.
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
	allContainers, err := h.containers.ListAllDiscoveredContainers(r.Context())
	if err != nil {
		// Non-fatal — return tree without containers rather than erroring.
		allContainers = nil
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

	// Group containers by their infra_component_id.
	containersByEngine := make(map[string][]*models.DiscoveredContainer)
	for _, c := range allContainers {
		containersByEngine[c.InfraComponentID] = append(containersByEngine[c.InfraComponentID], c)
	}

	// Pass 1: apps grouped by parent component ID.
	appsByParent := make(map[string][]topologyApp)
	for _, link := range allLinks {
		if link.ChildType == "app" {
			if a, ok := appByID[link.ChildID]; ok {
				appsByParent[link.ParentID] = append(appsByParent[link.ParentID], topologyApp{
					ID:      a.ID,
					Name:    a.Name,
					IconURL: "/api/v1/icons/" + a.ProfileID,
				})
			}
		}
	}

	// Pass 2: infra component children grouped by parent ID.
	childrenByParent := make(map[string][]models.InfrastructureComponent)
	for _, link := range allLinks {
		if link.ChildType != "app" && link.ChildType != "container" {
			if c, ok := compByID[link.ChildID]; ok {
				childrenByParent[link.ParentID] = append(childrenByParent[link.ParentID], c)
			}
		}
	}

	// Roots = components not appearing as a non-app child.
	childIDs := make(map[string]bool)
	for _, link := range allLinks {
		if link.ChildType != "app" && link.ChildType != "container" {
			childIDs[link.ChildID] = true
		}
	}

	var buildNode func(c models.InfrastructureComponent) topologyComponent
	buildNode = func(c models.InfrastructureComponent) topologyComponent {
		apps := appsByParent[c.ID]
		if apps == nil {
			apps = []topologyApp{}
		}

		// Infra component children.
		childNodes := []topologyComponent{}
		for _, child := range childrenByParent[c.ID] {
			childNodes = append(childNodes, buildNode(child))
		}

		// Discovered containers as leaf children (docker_engine / portainer).
		// App linkage comes from component_links (parent_type=container, child_type=app),
		// already resolved into appsByParent keyed by the container's ID.
		for _, ct := range containersByEngine[c.ID] {
			node := topologyComponent{
				ID:       ct.ID,
				Name:     ct.ContainerName,
				Type:     "container",
				Status:   ct.Status,
				Children: []topologyComponent{},
				Apps:     []topologyApp{},
			}
			if linked := appsByParent[ct.ID]; len(linked) > 0 {
				node.AppID      = linked[0].ID
				node.AppName    = linked[0].Name
				node.AppIconURL = linked[0].IconURL
			}
			childNodes = append(childNodes, node)
		}

		meta := ""
		if c.Meta != nil {
			meta = *c.Meta
		}
		return topologyComponent{
			ID:       c.ID,
			Name:     c.Name,
			Type:     c.Type,
			Status:   c.LastStatus,
			IP:       c.IP,
			Notes:    c.Notes,
			Meta:     meta,
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
