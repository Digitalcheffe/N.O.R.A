package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/digitalcheffe/nora/internal/jobs"
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
	store      *repo.Store
}

// NewInfraComponentHandler returns a handler wired to the given repos.
func NewInfraComponentHandler(components repo.InfraComponentRepo, rollups repo.ResourceRollupRepo, checks repo.CheckRepo, traefik repo.TraefikComponentRepo, store *repo.Store) *InfraComponentHandler {
	return &InfraComponentHandler{components: components, rollups: rollups, checks: checks, traefik: traefik, store: store}
}

// Routes registers all infrastructure component endpoints on r.
func (h *InfraComponentHandler) Routes(r chi.Router) {
	r.Get("/infrastructure", h.List)
	r.Post("/infrastructure", h.Create)
	r.Get("/infrastructure/{id}", h.Get)
	r.Put("/infrastructure/{id}", h.Update)
	r.Delete("/infrastructure/{id}", h.Delete)
	r.Post("/infrastructure/{id}/scan", h.Scan)
	r.Get("/infrastructure/{id}/resources", h.GetResources)
	r.Get("/infrastructure/{id}/resources/history", h.GetResourceHistory)
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
// CredentialMeta returns the non-secret fields of the stored credentials so
// the edit form can pre-populate them without exposing secrets.
type infraComponentResponse struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	IP               string                 `json:"ip"`
	Type             string                 `json:"type"`
	CollectionMethod string                 `json:"collection_method"`
	ParentID         *string                `json:"parent_id,omitempty"`
	SNMPConfig       *string                `json:"snmp_config,omitempty"`
	Notes            string                 `json:"notes"`
	Enabled          bool                   `json:"enabled"`
	LastPolledAt     *string                `json:"last_polled_at,omitempty"`
	LastStatus       string                 `json:"last_status"`
	CreatedAt        string                 `json:"created_at"`
	HasCredentials   bool                   `json:"has_credentials"`
	CredentialMeta   map[string]interface{} `json:"credential_meta,omitempty"`
}

// credentialSecretFields maps component type to the secret key(s) that must
// never be returned to the client.
var credentialSecretFields = map[string][]string{
	"proxmox_node": {"token_secret"},
	"synology":     {"password"},
	"traefik":      {"api_key"},
}

// extractCredentialMeta parses stored credentials and returns a copy with
// secret fields removed.
func extractCredentialMeta(compType, credJSON string) map[string]interface{} {
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(credJSON), &raw); err != nil {
		return nil
	}
	secrets := credentialSecretFields[compType]
	for _, k := range secrets {
		delete(raw, k)
	}
	if len(raw) == 0 {
		return nil
	}
	return raw
}

// mergeCredentials ensures that if the incoming credential JSON omits or leaves
// blank a secret field, the existing stored secret is preserved.
func mergeCredentials(existing *string, incoming *string, compType string) *string {
	secrets := credentialSecretFields[compType]
	if len(secrets) == 0 || existing == nil || *existing == "" || incoming == nil {
		return incoming
	}
	var existingMap, incomingMap map[string]interface{}
	if err := json.Unmarshal([]byte(*existing), &existingMap); err != nil {
		return incoming
	}
	if err := json.Unmarshal([]byte(*incoming), &incomingMap); err != nil {
		return incoming
	}
	changed := false
	for _, k := range secrets {
		v, ok := incomingMap[k]
		if !ok || v == nil || v == "" {
			if existingVal, has := existingMap[k]; has {
				incomingMap[k] = existingVal
				changed = true
			}
		}
	}
	if !changed {
		return incoming
	}
	merged, err := json.Marshal(incomingMap)
	if err != nil {
		return incoming
	}
	s := string(merged)
	return &s
}

func toResponse(c *models.InfrastructureComponent) infraComponentResponse {
	resp := infraComponentResponse{
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
		HasCredentials:   c.Credentials != nil && *c.Credentials != "",
	}
	if resp.HasCredentials {
		resp.CredentialMeta = extractCredentialMeta(c.Type, *c.Credentials)
	}
	return resp
}

// ── validation ────────────────────────────────────────────────────────────────

var validComponentTypes = map[string]bool{
	"proxmox_node":  true,
	"synology":      true,
	"vm":            true,
	"lxc":           true,
	"bare_metal":    true,
	"linux_host":    true,
	"windows_host":  true,
	"generic_host":  true,
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
		existing.Credentials = mergeCredentials(existing.Credentials, req.Credentials, existing.Type)
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

// scanResult is the response shape for POST /infrastructure/{id}/scan.
type scanResult struct {
	ComponentID string `json:"component_id"`
	Status      string `json:"status"`
	LastPolledAt string `json:"last_polled_at"`
	Message     string `json:"message,omitempty"`
	Error       string `json:"error,omitempty"`
}

// Scan immediately runs the appropriate poller for a single infrastructure
// component and returns the resulting status.
// POST /api/v1/infrastructure/{id}/scan
func (h *InfraComponentHandler) Scan(w http.ResponseWriter, r *http.Request) {
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

	// Use a detached context with a hard timeout so the poll isn't killed
	// if the browser closes the connection before the scan finishes.
	scanCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	status, scanErr := jobs.ScanOneComponent(scanCtx, h.store, c)

	now := time.Now().UTC().Format(time.RFC3339)
	resp := scanResult{
		ComponentID:  id,
		Status:       status,
		LastPolledAt: now,
	}
	if scanErr != nil {
		resp.Error = scanErr.Error()
		resp.Message = "scan failed — check credentials and connectivity"
	} else {
		resp.Message = "scan complete"
	}
	writeJSON(w, http.StatusOK, resp)
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
		// No rollup exists yet (hourly job hasn't run since first scan).
		// Fall back to raw resource_readings from the last hour so Scan Now
		// shows data immediately without waiting for the rollup cycle.
		now := time.Now().UTC()
		aggs, aggErr := h.rollups.AggregateReadings(r.Context(), now.Add(-time.Hour), now)
		if aggErr != nil || len(aggs) == 0 {
			resp.NoData = true
			writeJSON(w, http.StatusOK, resp)
			return
		}
		hasData := false
		for _, a := range aggs {
			if a.SourceID != id {
				continue
			}
			hasData = true
			switch {
			case a.Metric == "cpu_percent":
				resp.CPUPercent = a.Avg
			case a.Metric == "mem_percent":
				resp.MemPercent = a.Avg
			case a.Metric == "disk_percent":
				resp.DiskPercent = a.Avg
			case strings.HasPrefix(a.Metric, "disk_percent_"):
				volName := strings.TrimPrefix(a.Metric, "disk_percent_")
				resp.Volumes = append(resp.Volumes, VolumeResource{Name: volName, Percent: a.Avg})
			}
		}
		if !hasData {
			resp.NoData = true
			writeJSON(w, http.StatusOK, resp)
			return
		}
		resp.RecordedAt = now.Format(time.RFC3339)
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

// ── Resource history ──────────────────────────────────────────────────────────

// resourceHistoryPoint is one period bucket in the history response.
type resourceHistoryPoint struct {
	PeriodStart string  `json:"period_start"`
	Metric      string  `json:"metric"`
	Avg         float64 `json:"avg"`
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
}

type resourceHistoryResponse struct {
	ComponentID string                 `json:"component_id"`
	Period      string                 `json:"period"`
	Data        []resourceHistoryPoint `json:"data"`
	Total       int                    `json:"total"`
}

// GetResourceHistory returns historical rollup data for an infrastructure component.
// GET /api/v1/infrastructure/{id}/resources/history?period=hour|day&limit=24
func (h *InfraComponentHandler) GetResourceHistory(w http.ResponseWriter, r *http.Request) {
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

	limit := 24
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		var parsed int
		if _, err := fmt.Sscanf(lStr, "%d", &parsed); err == nil && parsed > 0 && parsed <= 168 {
			limit = parsed
		}
	}

	rollups, err := h.rollups.HistoryForSource(r.Context(), id, comp.Type, period, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	points := make([]resourceHistoryPoint, len(rollups))
	for i, rr := range rollups {
		points[i] = resourceHistoryPoint{
			PeriodStart: rr.PeriodStart.UTC().Format(time.RFC3339),
			Metric:      rr.Metric,
			Avg:         rr.Avg,
			Min:         rr.Min,
			Max:         rr.Max,
		}
	}

	writeJSON(w, http.StatusOK, resourceHistoryResponse{
		ComponentID: id,
		Period:      period,
		Data:        points,
		Total:       len(points),
	})
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
