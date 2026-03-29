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

// InfraComponentHandler handles CRUD for infrastructure_components.
type InfraComponentHandler struct {
	components repo.InfraComponentRepo
	rollups    repo.ResourceRollupRepo
}

// NewInfraComponentHandler returns a handler wired to the given repos.
func NewInfraComponentHandler(components repo.InfraComponentRepo, rollups repo.ResourceRollupRepo) *InfraComponentHandler {
	return &InfraComponentHandler{components: components, rollups: rollups}
}

// Routes registers all infrastructure component endpoints on r.
func (h *InfraComponentHandler) Routes(r chi.Router) {
	r.Get("/infrastructure", h.List)
	r.Post("/infrastructure", h.Create)
	r.Get("/infrastructure/{id}", h.Get)
	r.Put("/infrastructure/{id}", h.Update)
	r.Delete("/infrastructure/{id}", h.Delete)
	r.Get("/infrastructure/{id}/resources", h.GetResources)
}

// ── request / response types ──────────────────────────────────────────────────

type infraComponentRequest struct {
	Name             string  `json:"name"`
	IP               string  `json:"ip"`
	Type             string  `json:"type"`
	CollectionMethod string  `json:"collection_method"`
	ParentID         *string `json:"parent_id"`
	SNMPConfig       *string `json:"snmp_config"`
	Notes            string  `json:"notes"`
	Enabled          *bool   `json:"enabled"`
	// Credentials is accepted on create/update but never returned.
	Credentials *string `json:"credentials"`
}

// infraComponentResponse is InfrastructureComponent with Credentials stripped.
type infraComponentResponse struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	IP               string  `json:"ip"`
	Type             string  `json:"type"`
	CollectionMethod string  `json:"collection_method"`
	ParentID         *string `json:"parent_id,omitempty"`
	SNMPConfig       *string `json:"snmp_config,omitempty"`
	Notes            string  `json:"notes"`
	Enabled          bool    `json:"enabled"`
	LastPolledAt     *string `json:"last_polled_at,omitempty"`
	LastStatus       string  `json:"last_status"`
	CreatedAt        string  `json:"created_at"`
}

func toResponse(c *models.InfrastructureComponent) infraComponentResponse {
	return infraComponentResponse{
		ID:               c.ID,
		Name:             c.Name,
		IP:               c.IP,
		Type:             c.Type,
		CollectionMethod: c.CollectionMethod,
		ParentID:         c.ParentID,
		SNMPConfig:       c.SNMPConfig,
		Notes:            c.Notes,
		Enabled:          c.Enabled,
		LastPolledAt:     c.LastPolledAt,
		LastStatus:       c.LastStatus,
		CreatedAt:        c.CreatedAt,
	}
}

// ── validation ────────────────────────────────────────────────────────────────

var validComponentTypes = map[string]bool{
	"proxmox_node":  true,
	"synology":      true,
	"vm":            true,
	"lxc":           true,
	"bare_metal":    true,
	"windows_host":  true,
	"docker_engine": true,
}

var validCollectionMethods = map[string]bool{
	"proxmox_api":   true,
	"synology_api":  true,
	"snmp":          true,
	"docker_socket": true,
	"none":          true,
}

// ── handlers ──────────────────────────────────────────────────────────────────

// List returns all infrastructure components.
// GET /api/v1/infrastructure
func (h *InfraComponentHandler) List(w http.ResponseWriter, r *http.Request) {
	components, err := h.components.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result := make([]infraComponentResponse, len(components))
	for i, c := range components {
		result[i] = toResponse(&c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result, "total": len(result)})
}

// Create creates a new infrastructure component.
// POST /api/v1/infrastructure
func (h *InfraComponentHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req infraComponentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !validComponentTypes[req.Type] {
		writeError(w, http.StatusBadRequest, "invalid type")
		return
	}
	cm := req.CollectionMethod
	if cm == "" {
		cm = "none"
	}
	if !validCollectionMethods[cm] {
		writeError(w, http.StatusBadRequest, "invalid collection_method")
		return
	}

	// Validate parent_id if provided.
	if req.ParentID != nil && *req.ParentID != "" {
		if _, err := h.components.Get(r.Context(), *req.ParentID); errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusUnprocessableEntity, "parent_id not found")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	c := &models.InfrastructureComponent{
		ID:               uuid.New().String(),
		Name:             req.Name,
		IP:               req.IP,
		Type:             req.Type,
		CollectionMethod: cm,
		ParentID:         req.ParentID,
		Credentials:      req.Credentials,
		SNMPConfig:       req.SNMPConfig,
		Notes:            req.Notes,
		Enabled:          enabled,
		LastStatus:       "unknown",
		CreatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := h.components.Create(r.Context(), c); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toResponse(c))
}

// Get returns a single infrastructure component.
// GET /api/v1/infrastructure/{id}
func (h *InfraComponentHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	c, err := h.components.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "component not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toResponse(c))
}

// Update replaces mutable fields on an infrastructure component.
// PUT /api/v1/infrastructure/{id}
func (h *InfraComponentHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := h.components.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "component not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var req infraComponentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Type != "" && !validComponentTypes[req.Type] {
		writeError(w, http.StatusBadRequest, "invalid type")
		return
	}
	if req.CollectionMethod != "" && !validCollectionMethods[req.CollectionMethod] {
		writeError(w, http.StatusBadRequest, "invalid collection_method")
		return
	}
	if req.ParentID != nil && *req.ParentID != "" {
		if _, err := h.components.Get(r.Context(), *req.ParentID); errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusUnprocessableEntity, "parent_id not found")
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
	if req.CollectionMethod != "" {
		existing.CollectionMethod = req.CollectionMethod
	}
	existing.ParentID = req.ParentID
	if req.SNMPConfig != nil {
		existing.SNMPConfig = req.SNMPConfig
	}
	if req.Credentials != nil {
		existing.Credentials = req.Credentials
	}
	existing.Notes = req.Notes
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}

	if err := h.components.Update(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toResponse(existing))
}

// Delete removes an infrastructure component.
// DELETE /api/v1/infrastructure/{id}
func (h *InfraComponentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.components.Delete(r.Context(), id); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "component not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GetResources returns the latest resource rollup values for an infrastructure component.
// GET /api/v1/infrastructure/{id}/resources?period=hour|day
func (h *InfraComponentHandler) GetResources(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.components.Get(r.Context(), id); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "component not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "hour"
	}
	if period != "hour" && period != "day" {
		writeError(w, http.StatusBadRequest, "period must be hour or day")
		return
	}

	rollups, err := h.rollups.LatestForSource(r.Context(), id, "host", period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := hostResourcesResponse{}
	for _, rr := range rollups {
		switch rr.Metric {
		case "cpu_percent":
			resp.CPU = rr.Avg
		case "mem_percent":
			resp.Mem = rr.Avg
		case "disk_percent":
			resp.Disk = rr.Avg
		}
	}
	writeJSON(w, http.StatusOK, resp)
}
