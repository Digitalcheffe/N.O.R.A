package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
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
	checks     repo.CheckRepo
	traefik    repo.TraefikComponentRepo
}

// NewInfraComponentHandler returns a handler wired to the given repos.
func NewInfraComponentHandler(components repo.InfraComponentRepo, rollups repo.ResourceRollupRepo, checks repo.CheckRepo, traefik repo.TraefikComponentRepo) *InfraComponentHandler {
	return &InfraComponentHandler{components: components, rollups: rollups, checks: checks, traefik: traefik}
}

// Routes registers all infrastructure component endpoints on r.
func (h *InfraComponentHandler) Routes(r chi.Router) {
	r.Get("/infrastructure", h.List)
	r.Post("/infrastructure", h.Create)
	r.Get("/infrastructure/{id}", h.Get)
	r.Put("/infrastructure/{id}", h.Update)
	r.Delete("/infrastructure/{id}", h.Delete)
	r.Get("/infrastructure/{id}/resources", h.GetResources)
	r.Get("/infrastructure/{id}/traefik", h.GetTraefikDetail)
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
	"traefik":       true,
}

var validCollectionMethods = map[string]bool{
	"proxmox_api":   true,
	"synology_api":  true,
	"snmp":          true,
	"docker_socket": true,
	"traefik_api":   true,
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

// Delete removes an infrastructure component and any owned SSL checks.
// DELETE /api/v1/infrastructure/{id}
func (h *InfraComponentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	comp, err := h.components.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "component not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// For traefik components, delete owned SSL checks before removing the component.
	// traefik_component_certs and traefik_routes cascade automatically via FK.
	if comp.Type == "traefik" {
		if err := h.checks.DeleteBySourceComponent(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if err := h.components.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// VolumeResource holds per-volume disk utilisation for Synology and similar components.
type VolumeResource struct {
	Name    string  `json:"name"`
	Percent float64 `json:"percent"`
}

// infraResourcesResponse is the response shape for GET /infrastructure/{id}/resources.
type infraResourcesResponse struct {
	ComponentID string           `json:"component_id"`
	Period      string           `json:"period"`
	CPUPercent  float64          `json:"cpu_percent"`
	MemPercent  float64          `json:"mem_percent"`
	DiskPercent float64          `json:"disk_percent"`
	Volumes     []VolumeResource `json:"volumes,omitempty"`
	RecordedAt  string           `json:"recorded_at,omitempty"`
	NoData      bool             `json:"no_data,omitempty"`
}

// GetResources returns the latest resource rollup values for an infrastructure component.
// GET /api/v1/infrastructure/{id}/resources?period=hour|day
func (h *InfraComponentHandler) GetResources(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	comp, err := h.components.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
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

	rollups, err := h.rollups.LatestForSource(r.Context(), id, comp.Type, period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := infraResourcesResponse{
		ComponentID: id,
		Period:      period,
	}
	if len(rollups) == 0 {
		resp.NoData = true
		writeJSON(w, http.StatusOK, resp)
		return
	}

	for _, rr := range rollups {
		switch {
		case rr.Metric == "cpu_percent":
			resp.CPUPercent = rr.Avg
			if resp.RecordedAt == "" {
				resp.RecordedAt = rr.PeriodStart.UTC().Format(time.RFC3339)
			}
		case rr.Metric == "mem_percent":
			resp.MemPercent = rr.Avg
		case rr.Metric == "disk_percent":
			resp.DiskPercent = rr.Avg
		case strings.HasPrefix(rr.Metric, "disk_percent_"):
			volName := strings.TrimPrefix(rr.Metric, "disk_percent_")
			resp.Volumes = append(resp.Volumes, VolumeResource{Name: volName, Percent: rr.Avg})
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// ── Traefik detail ────────────────────────────────────────────────────────────

// traefikCertWithCheck extends TraefikComponentCert with the matching SSL check status.
type traefikCertWithCheck struct {
	ID          string  `json:"id"`
	Domain      string  `json:"domain"`
	Issuer      *string `json:"issuer,omitempty"`
	ExpiresAt   *string `json:"expires_at,omitempty"`
	SANs        []string `json:"sans"`
	LastSeenAt  string  `json:"last_seen_at"`
	CheckStatus string  `json:"check_status"` // "", "up", "warn", "down", "critical"
	CheckID     string  `json:"check_id,omitempty"`
}

type traefikDetailResponse struct {
	ComponentID string                 `json:"component_id"`
	CertCount   int                    `json:"cert_count"`
	WarnCount   int                    `json:"warn_count"`
	CritCount   int                    `json:"crit_count"`
	Certs       []traefikCertWithCheck `json:"certs"`
	Routes      []models.TraefikRoute  `json:"routes"`
}

// GetTraefikDetail returns certs, routes, and SSL check status for a Traefik component.
// GET /api/v1/infrastructure/{id}/traefik
func (h *InfraComponentHandler) GetTraefikDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	comp, err := h.components.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "component not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if comp.Type != "traefik" {
		writeError(w, http.StatusBadRequest, "component is not a traefik type")
		return
	}

	certs, err := h.traefik.ListCerts(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	routes, err := h.traefik.ListRoutes(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Build a map of SSL checks owned by this component, keyed by target (domain).
	ownedChecks, err := h.checks.ListBySourceComponent(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	checkByDomain := make(map[string]models.MonitorCheck, len(ownedChecks))
	for _, ch := range ownedChecks {
		checkByDomain[ch.Target] = ch
	}

	now := time.Now().UTC()
	warnDays := 30
	critDays := 7

	certItems := make([]traefikCertWithCheck, len(certs))
	warnCount, critCount := 0, 0
	for i, c := range certs {
		item := traefikCertWithCheck{
			ID:         c.ID,
			Domain:     c.Domain,
			Issuer:     c.Issuer,
			SANs:       c.SANs,
			LastSeenAt: c.LastSeenAt.UTC().Format(time.RFC3339),
		}
		if c.ExpiresAt != nil {
			s := c.ExpiresAt.UTC().Format(time.RFC3339)
			item.ExpiresAt = &s
			days := int(c.ExpiresAt.Sub(now).Hours() / 24)
			if days <= critDays {
				critCount++
			} else if days <= warnDays {
				warnCount++
			}
		}
		if ch, ok := checkByDomain[c.Domain]; ok {
			item.CheckStatus = ch.LastStatus
			item.CheckID = ch.ID
		}
		certItems[i] = item
	}

	writeJSON(w, http.StatusOK, traefikDetailResponse{
		ComponentID: id,
		CertCount:   len(certs),
		WarnCount:   warnCount,
		CritCount:   critCount,
		Certs:       certItems,
		Routes:      routes,
	})
}
