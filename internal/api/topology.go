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

// TopologyHandler holds dependencies for all topology endpoints.
type TopologyHandler struct {
	physicalHosts repo.PhysicalHostRepo
	virtualHosts  repo.VirtualHostRepo
	dockerEngines repo.DockerEngineRepo
	apps          repo.AppRepo
}

// NewTopologyHandler creates a TopologyHandler.
func NewTopologyHandler(ph repo.PhysicalHostRepo, vh repo.VirtualHostRepo, de repo.DockerEngineRepo, apps repo.AppRepo) *TopologyHandler {
	return &TopologyHandler{
		physicalHosts: ph,
		virtualHosts:  vh,
		dockerEngines: de,
		apps:          apps,
	}
}

// Routes registers all topology endpoints on r.
func (h *TopologyHandler) Routes(r chi.Router) {
	// Physical hosts
	r.Get("/hosts/physical", h.ListPhysical)
	r.Post("/hosts/physical", h.CreatePhysical)
	r.Get("/hosts/physical/{id}", h.GetPhysical)
	r.Put("/hosts/physical/{id}", h.UpdatePhysical)
	r.Delete("/hosts/physical/{id}", h.DeletePhysical)

	// Virtual hosts
	r.Get("/hosts/virtual", h.ListVirtual)
	r.Post("/hosts/virtual", h.CreateVirtual)
	r.Get("/hosts/virtual/{id}", h.GetVirtual)
	r.Put("/hosts/virtual/{id}", h.UpdateVirtual)
	r.Delete("/hosts/virtual/{id}", h.DeleteVirtual)

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

type physicalHostRequest struct {
	Name  string `json:"name"`
	IP    string `json:"ip"`
	Type  string `json:"type"`
	Notes string `json:"notes"`
}

type physicalHostResponse struct {
	models.PhysicalHost
	VirtualHostIDs []string `json:"virtual_hosts"`
}

type virtualHostRequest struct {
	Name           string `json:"name"`
	IP             string `json:"ip"`
	Type           string `json:"type"`
	PhysicalHostID string `json:"physical_host_id"`
}

type virtualHostResponse struct {
	models.VirtualHost
	DockerEngineIDs []string `json:"docker_engines"`
}

type dockerEngineRequest struct {
	Name          string `json:"name"`
	SocketType    string `json:"socket_type"`
	SocketPath    string `json:"socket_path"`
	VirtualHostID string `json:"virtual_host_id"`
}

// topology chain response types

type topologyApp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type topologyEngine struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	DockerEngines []topologyApp `json:"apps"`
}

type topologyVirtualHost struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	DockerEngines []topologyEngine `json:"docker_engines"`
}

type topologyPhysicalHost struct {
	ID           string                `json:"id"`
	Name         string                `json:"name"`
	Type         string                `json:"type"`
	VirtualHosts []topologyVirtualHost `json:"virtual_hosts"`
}

// ---- validation helpers -----------------------------------------------------

var validPhysicalTypes = map[string]bool{"bare_metal": true, "proxmox_node": true}
var validVirtualTypes = map[string]bool{"vm": true, "lxc": true, "wsl": true}
var validSocketTypes = map[string]bool{"local": true, "remote_proxy": true}

// ---- physical host handlers -------------------------------------------------

// ListPhysical returns all physical hosts with nested virtual host IDs.
// GET /api/v1/hosts/physical
func (h *TopologyHandler) ListPhysical(w http.ResponseWriter, r *http.Request) {
	hosts, err := h.physicalHosts.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	vhosts, err := h.virtualHosts.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Group virtual host IDs by physical_host_id.
	vByPhysical := make(map[string][]string)
	for _, v := range vhosts {
		if v.PhysicalHostID != "" {
			vByPhysical[v.PhysicalHostID] = append(vByPhysical[v.PhysicalHostID], v.ID)
		}
	}

	result := make([]physicalHostResponse, 0, len(hosts))
	for _, ph := range hosts {
		ids := vByPhysical[ph.ID]
		if ids == nil {
			ids = []string{}
		}
		result = append(result, physicalHostResponse{PhysicalHost: ph, VirtualHostIDs: ids})
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": result, "total": len(result)})
}

// CreatePhysical creates a physical host.
// POST /api/v1/hosts/physical
func (h *TopologyHandler) CreatePhysical(w http.ResponseWriter, r *http.Request) {
	var req physicalHostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.IP == "" {
		writeError(w, http.StatusBadRequest, "ip is required")
		return
	}
	if !validPhysicalTypes[req.Type] {
		writeError(w, http.StatusBadRequest, "type must be bare_metal or proxmox_node")
		return
	}

	host := &models.PhysicalHost{
		ID:        uuid.New().String(),
		Name:      req.Name,
		IP:        req.IP,
		Type:      req.Type,
		Notes:     req.Notes,
		CreatedAt: time.Now().UTC(),
	}
	if err := h.physicalHosts.Create(r.Context(), host); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, host)
}

// GetPhysical returns a single physical host.
// GET /api/v1/hosts/physical/{id}
func (h *TopologyHandler) GetPhysical(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	host, err := h.physicalHosts.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "physical host not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, host)
}

// UpdatePhysical replaces mutable fields on a physical host.
// PUT /api/v1/hosts/physical/{id}
func (h *TopologyHandler) UpdatePhysical(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := h.physicalHosts.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "physical host not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var req physicalHostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Type != "" && !validPhysicalTypes[req.Type] {
		writeError(w, http.StatusBadRequest, "type must be bare_metal or proxmox_node")
		return
	}

	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.IP != "" {
		existing.IP = req.IP
	}
	if req.Type != "" {
		existing.Type = req.Type
	}
	existing.Notes = req.Notes

	if err := h.physicalHosts.Update(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

// DeletePhysical removes a physical host (virtual hosts are SET NULL, not cascaded).
// DELETE /api/v1/hosts/physical/{id}
func (h *TopologyHandler) DeletePhysical(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.physicalHosts.Delete(r.Context(), id); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "physical host not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- virtual host handlers --------------------------------------------------

// ListVirtual returns all virtual hosts with nested docker engine IDs.
// GET /api/v1/hosts/virtual
func (h *TopologyHandler) ListVirtual(w http.ResponseWriter, r *http.Request) {
	hosts, err := h.virtualHosts.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	engines, err := h.dockerEngines.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Group docker engine IDs by virtual_host_id.
	deByVirtual := make(map[string][]string)
	for _, e := range engines {
		if e.VirtualHostID != "" {
			deByVirtual[e.VirtualHostID] = append(deByVirtual[e.VirtualHostID], e.ID)
		}
	}

	result := make([]virtualHostResponse, 0, len(hosts))
	for _, vh := range hosts {
		ids := deByVirtual[vh.ID]
		if ids == nil {
			ids = []string{}
		}
		result = append(result, virtualHostResponse{VirtualHost: vh, DockerEngineIDs: ids})
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": result, "total": len(result)})
}

// CreateVirtual creates a virtual host.
// POST /api/v1/hosts/virtual
func (h *TopologyHandler) CreateVirtual(w http.ResponseWriter, r *http.Request) {
	var req virtualHostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.IP == "" {
		writeError(w, http.StatusBadRequest, "ip is required")
		return
	}
	if !validVirtualTypes[req.Type] {
		writeError(w, http.StatusBadRequest, "type must be vm, lxc, or wsl")
		return
	}

	// Validate FK if provided.
	if req.PhysicalHostID != "" {
		if _, err := h.physicalHosts.Get(r.Context(), req.PhysicalHostID); errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusUnprocessableEntity, "physical_host_id not found")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	host := &models.VirtualHost{
		ID:             uuid.New().String(),
		PhysicalHostID: req.PhysicalHostID,
		Name:           req.Name,
		IP:             req.IP,
		Type:           req.Type,
		CreatedAt:      time.Now().UTC(),
	}
	if err := h.virtualHosts.Create(r.Context(), host); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, host)
}

// GetVirtual returns a single virtual host.
// GET /api/v1/hosts/virtual/{id}
func (h *TopologyHandler) GetVirtual(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	host, err := h.virtualHosts.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "virtual host not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, host)
}

// UpdateVirtual replaces mutable fields on a virtual host.
// PUT /api/v1/hosts/virtual/{id}
func (h *TopologyHandler) UpdateVirtual(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := h.virtualHosts.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "virtual host not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var req virtualHostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Type != "" && !validVirtualTypes[req.Type] {
		writeError(w, http.StatusBadRequest, "type must be vm, lxc, or wsl")
		return
	}
	if req.PhysicalHostID != "" {
		if _, err := h.physicalHosts.Get(r.Context(), req.PhysicalHostID); errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusUnprocessableEntity, "physical_host_id not found")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.IP != "" {
		existing.IP = req.IP
	}
	if req.Type != "" {
		existing.Type = req.Type
	}
	existing.PhysicalHostID = req.PhysicalHostID

	if err := h.virtualHosts.Update(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

// DeleteVirtual removes a virtual host.
// DELETE /api/v1/hosts/virtual/{id}
func (h *TopologyHandler) DeleteVirtual(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.virtualHosts.Delete(r.Context(), id); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "virtual host not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

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

	// Validate FK if provided.
	if req.VirtualHostID != "" {
		if _, err := h.virtualHosts.Get(r.Context(), req.VirtualHostID); errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusUnprocessableEntity, "virtual_host_id not found")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	engine := &models.DockerEngine{
		ID:            uuid.New().String(),
		VirtualHostID: req.VirtualHostID,
		Name:          req.Name,
		SocketType:    req.SocketType,
		SocketPath:    req.SocketPath,
		CreatedAt:     time.Now().UTC(),
	}
	if err := h.dockerEngines.Create(r.Context(), engine); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
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
	if req.VirtualHostID != "" {
		if _, err := h.virtualHosts.Get(r.Context(), req.VirtualHostID); errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusUnprocessableEntity, "virtual_host_id not found")
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
	existing.VirtualHostID = req.VirtualHostID

	if err := h.dockerEngines.Update(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
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

// GetTopology returns the full physical → virtual → docker → app chain.
// GET /api/v1/topology
func (h *TopologyHandler) GetTopology(w http.ResponseWriter, r *http.Request) {
	physHosts, err := h.physicalHosts.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	virtHosts, err := h.virtualHosts.List(r.Context())
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

	// Index apps by docker_engine_id.
	appsByEngine := make(map[string][]topologyApp)
	for _, a := range allApps {
		if a.DockerEngineID != "" {
			appsByEngine[a.DockerEngineID] = append(appsByEngine[a.DockerEngineID], topologyApp{ID: a.ID, Name: a.Name})
		}
	}

	// Index engines by virtual_host_id.
	enginesByVirtual := make(map[string][]topologyEngine)
	for _, e := range engines {
		apps := appsByEngine[e.ID]
		if apps == nil {
			apps = []topologyApp{}
		}
		enginesByVirtual[e.VirtualHostID] = append(enginesByVirtual[e.VirtualHostID], topologyEngine{
			ID:            e.ID,
			Name:          e.Name,
			DockerEngines: apps,
		})
	}

	// Index virtual hosts by physical_host_id.
	virtByPhysical := make(map[string][]topologyVirtualHost)
	for _, v := range virtHosts {
		des := enginesByVirtual[v.ID]
		if des == nil {
			des = []topologyEngine{}
		}
		virtByPhysical[v.PhysicalHostID] = append(virtByPhysical[v.PhysicalHostID], topologyVirtualHost{
			ID:            v.ID,
			Name:          v.Name,
			DockerEngines: des,
		})
	}

	result := make([]topologyPhysicalHost, 0, len(physHosts))
	for _, p := range physHosts {
		vhs := virtByPhysical[p.ID]
		if vhs == nil {
			vhs = []topologyVirtualHost{}
		}
		result = append(result, topologyPhysicalHost{
			ID:           p.ID,
			Name:         p.Name,
			Type:         p.Type,
			VirtualHosts: vhs,
		})
	}

	writeJSON(w, http.StatusOK, result)
}
