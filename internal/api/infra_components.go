package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/digitalcheffe/nora/internal/infra"
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
	events     repo.EventRepo
	store      *repo.Store
}

// NewInfraComponentHandler returns a handler wired to the given repos.
func NewInfraComponentHandler(components repo.InfraComponentRepo, rollups repo.ResourceRollupRepo, checks repo.CheckRepo, events repo.EventRepo, store *repo.Store) *InfraComponentHandler {
	return &InfraComponentHandler{components: components, rollups: rollups, checks: checks, events: events, store: store}
}

// Routes registers all infrastructure component endpoints on r.
func (h *InfraComponentHandler) Routes(r chi.Router) {
	r.Get("/infrastructure", h.List)
	r.Post("/infrastructure", h.Create)
	r.Get("/infrastructure/{id}", h.Get)
	r.Put("/infrastructure/{id}", h.Update)
	r.Delete("/infrastructure/{id}", h.Delete)
	r.Post("/infrastructure/{id}/scan", h.Scan)
	r.Post("/infrastructure/{id}/discover", h.Discover)
	r.Get("/infrastructure/{id}/resources", h.GetResources)
	r.Get("/infrastructure/{id}/resources/history", h.GetResourceHistory)
	r.Get("/infrastructure/{id}/snmp", h.GetSNMPDetail)
	r.Get("/infrastructure/{id}/synology", h.GetSynologyDetail)
	r.Get("/infrastructure/{id}/traefik/overview", h.GetTraefikOverview)
	r.Get("/infrastructure/{id}/traefik/routers", h.GetTraefikRouters)
	r.Get("/infrastructure/{id}/traefik/services", h.GetTraefikServices)
	r.Get("/infrastructure/{id}/events", h.ListEvents)
	r.Get("/infrastructure/{id}/children", h.ListChildren)
	r.Get("/infrastructure/{id}/apps", h.ListLinkedApps)
	r.Post("/infrastructure/{id}/apps/{appID}", h.LinkApp)
	r.Delete("/infrastructure/{id}/apps/{appID}", h.UnlinkApp)
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
	"portainer":    {"api_key"},
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
	"portainer":     true,
}

var validCollectionMethods = map[string]bool{
	"proxmox_api":   true,
	"synology_api":  true,
	"snmp":          true,
	"docker_socket": true,
	"traefik_api":   true,
	"portainer_api": true,
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

// discoverResponse is the response shape for POST /infrastructure/{id}/discover.
type discoverResponse struct {
	Status     string `json:"status"`
	Discovered int    `json:"discovered"`
	Updated    int    `json:"updated"`
	Missing    int    `json:"missing"`
	Message    string `json:"message,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Discover immediately runs the discovery scanner for a single infrastructure
// component and returns found/updated/missing counts.
// POST /api/v1/infrastructure/{id}/discover
func (h *InfraComponentHandler) Discover(w http.ResponseWriter, r *http.Request) {
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

	// Use a detached context with a hard timeout so the scan is not cancelled
	// if the client disconnects before it completes.
	discCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, discErr := jobs.DiscoverOneComponent(discCtx, h.store, c)
	if discErr != nil {
		// Write a failure event so the component's event feed reflects the attempt.
		_ = h.store.Events.Create(discCtx, &models.Event{
			ID:         uuid.New().String(),
			Level:      "error",
			SourceName: c.Name,
			SourceType: c.Type,
			SourceID:   c.ID,
			Title:      fmt.Sprintf("[discovery] Discovery failed — %s", discErr.Error()),
			Payload:    fmt.Sprintf(`{"bucket":"discovery","component_id":%q,"error":%q}`, c.ID, discErr.Error()),
			CreatedAt:  time.Now().UTC(),
		})
		writeJSON(w, http.StatusOK, discoverResponse{
			Status:  "error",
			Error:   discErr.Error(),
			Message: "discovery failed — check credentials and connectivity",
		})
		return
	}
	writeJSON(w, http.StatusOK, discoverResponse{
		Status:     "ok",
		Discovered: result.Found,
		Missing:    result.Disappeared,
		Message:    "discovery complete",
	})
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

	// SNMP pollers write resource_readings with source_type="snmp_host" regardless
	// of the component type (linux_host, bare_metal, etc.). Use the poller's
	// source_type when querying rollups so the lookup matches what was stored.
	sourceType := comp.Type
	if comp.CollectionMethod == "snmp" {
		sourceType = "snmp_host"
	}

	rollups, err := h.rollups.LatestForSource(r.Context(), id, sourceType, period)
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
		// Fall back to raw resource_readings from the last hour so Discover Now
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

	histSourceType := comp.Type
	if comp.CollectionMethod == "snmp" {
		histSourceType = "snmp_host"
	}
	rollups, err := h.rollups.HistoryForSource(r.Context(), id, histSourceType, period, limit)
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

// ── SNMP detail ───────────────────────────────────────────────────────────────

// snmpDetailMemory is the memory sub-object in the SNMP detail response.
type snmpDetailMemory struct {
	UsedBytes  int64   `json:"used_bytes"`
	TotalBytes int64   `json:"total_bytes"`
	Percent    float64 `json:"percent"`
}

// snmpDetailDisk is one disk entry in the SNMP detail response.
type snmpDetailDisk struct {
	Label      string  `json:"label"`
	UsedBytes  int64   `json:"used_bytes"`
	TotalBytes int64   `json:"total_bytes"`
	Percent    float64 `json:"percent"`
}

// snmpDetailResponse is returned by GET /api/v1/infrastructure/{id}/snmp.
type snmpDetailResponse struct {
	OSDescription string           `json:"os_description"`
	Uptime        string           `json:"uptime"`
	Hostname      string           `json:"hostname"`
	CPUPercent    float64          `json:"cpu_percent"`
	Memory        snmpDetailMemory `json:"memory"`
	Disks         []snmpDetailDisk `json:"disks"`
	NoData        bool             `json:"no_data,omitempty"`
}

// GetSNMPDetail returns the latest SNMP system identity and resource snapshot.
// GET /api/v1/infrastructure/{id}/snmp
func (h *InfraComponentHandler) GetSNMPDetail(w http.ResponseWriter, r *http.Request) {
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
	if comp.CollectionMethod != "snmp" {
		writeError(w, http.StatusBadRequest, "component does not use SNMP collection")
		return
	}

	// No poll has run yet — return a zeroed no_data response so the UI can render.
	if comp.SNMPMeta == nil || *comp.SNMPMeta == "" {
		resp := snmpDetailResponse{NoData: true, Disks: []snmpDetailDisk{}}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Parse snmp_meta snapshot written by the poller.
	var meta struct {
		OSDescription string  `json:"os_description"`
		Uptime        string  `json:"uptime"`
		Hostname      string  `json:"hostname"`
		CPUPercent    float64 `json:"cpu_percent"`
		Memory        struct {
			UsedBytes  int64   `json:"used_bytes"`
			TotalBytes int64   `json:"total_bytes"`
			Percent    float64 `json:"percent"`
		} `json:"memory"`
		Disks []struct {
			Label      string  `json:"label"`
			UsedBytes  int64   `json:"used_bytes"`
			TotalBytes int64   `json:"total_bytes"`
			Percent    float64 `json:"percent"`
		} `json:"disks"`
	}
	if jsonErr := json.Unmarshal([]byte(*comp.SNMPMeta), &meta); jsonErr != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse snmp_meta")
		return
	}

	disks := make([]snmpDetailDisk, len(meta.Disks))
	for i, d := range meta.Disks {
		disks[i] = snmpDetailDisk{
			Label:      d.Label,
			UsedBytes:  d.UsedBytes,
			TotalBytes: d.TotalBytes,
			Percent:    d.Percent,
		}
	}

	resp := snmpDetailResponse{
		OSDescription: meta.OSDescription,
		Uptime:        meta.Uptime,
		Hostname:      meta.Hostname,
		CPUPercent:    meta.CPUPercent,
		Memory: snmpDetailMemory{
			UsedBytes:  meta.Memory.UsedBytes,
			TotalBytes: meta.Memory.TotalBytes,
			Percent:    meta.Memory.Percent,
		},
		Disks: disks,
	}
	writeJSON(w, http.StatusOK, resp)
}

// ── Synology detail ───────────────────────────────────────────────────────────

// GetSynologyDetail returns the latest Synology DSM snapshot stored in synology_meta.
// GET /api/v1/infrastructure/{id}/synology
func (h *InfraComponentHandler) GetSynologyDetail(w http.ResponseWriter, r *http.Request) {
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
	if comp.Type != "synology" {
		writeError(w, http.StatusBadRequest, "component is not a Synology NAS")
		return
	}

	// No poll has run yet — return a zero-value no_data response so the UI renders.
	if comp.SynologyMeta == nil || *comp.SynologyMeta == "" {
		resp := infra.SynologyMeta{
			Volumes: []infra.SynologyVolume{},
			Disks:   []infra.SynologyDisk{},
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"data": resp, "no_data": true})
		return
	}

	var meta infra.SynologyMeta
	if jsonErr := json.Unmarshal([]byte(*comp.SynologyMeta), &meta); jsonErr != nil {
		writeError(w, http.StatusInternalServerError, "failed to parse synology_meta")
		return
	}
	if meta.Volumes == nil {
		meta.Volumes = []infra.SynologyVolume{}
	}
	if meta.Disks == nil {
		meta.Disks = []infra.SynologyDisk{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": meta})
}

// ── Children & linked-app endpoints ──────────────────────────────────────────

// ListChildren returns all infrastructure components whose parent_id matches id.
// GET /api/v1/infrastructure/{id}/children
func (h *InfraComponentHandler) ListChildren(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.components.Get(r.Context(), id); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "component not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	children, err := h.components.ListByParent(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Strip credentials from child records before returning.
	resp := make([]infraComponentResponse, len(children))
	for i, c := range children {
		resp[i] = toResponse(&c)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": resp, "total": len(resp)})
}

// ListLinkedApps returns all apps whose host_component_id matches id.
// GET /api/v1/infrastructure/{id}/apps
func (h *InfraComponentHandler) ListLinkedApps(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.components.Get(r.Context(), id); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "component not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	apps, err := h.store.Apps.ListByHost(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": apps, "total": len(apps)})
}

// LinkApp sets host_component_id on an app to link it to this component.
// POST /api/v1/infrastructure/{id}/apps/{appID}
func (h *InfraComponentHandler) LinkApp(w http.ResponseWriter, r *http.Request) {
	id    := chi.URLParam(r, "id")
	appID := chi.URLParam(r, "appID")

	if _, err := h.components.Get(r.Context(), id); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "component not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := h.store.Apps.SetHostComponentID(r.Context(), appID, &id); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UnlinkApp clears host_component_id on an app.
// DELETE /api/v1/infrastructure/{id}/apps/{appID}
func (h *InfraComponentHandler) UnlinkApp(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")

	if err := h.store.Apps.SetHostComponentID(r.Context(), appID, nil); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListEvents returns events whose fields.component_id matches the component.
// Accepts the same filter params as GET /events (severity, since, until, limit, offset, sort, search).
// GET /api/v1/infrastructure/{id}/events
func (h *InfraComponentHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	f, err := parseFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	f.SourceID = id

	evts, total, err := h.events.List(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]eventItem, len(evts))
	for i, e := range evts {
		items[i] = toEventItem(e)
	}
	writeJSON(w, http.StatusOK, listEventsResponse{
		Data:   items,
		Total:  total,
		Limit:  f.Limit,
		Offset: f.Offset,
	})
}

// ── Traefik expanded endpoints (Infra-10) ────────────────────────────────────

// GetTraefikOverview returns the latest traefik_overview row for the component.
// GET /api/v1/infrastructure/{id}/traefik/overview
func (h *InfraComponentHandler) GetTraefikOverview(w http.ResponseWriter, r *http.Request) {
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
	ov, err := h.store.TraefikOverview.Get(r.Context(), id)
	if err != nil {
		// Not polled yet — return a zeroed structure so the UI can render something.
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"component_id":      id,
			"version":           "",
			"routers_total":     0,
			"routers_errors":    0,
			"routers_warnings":  0,
			"services_total":    0,
			"services_errors":   0,
			"middlewares_total": 0,
			"updated_at":        nil,
		})
		return
	}
	writeJSON(w, http.StatusOK, ov)
}

// GetTraefikRouters returns all discovered_routes for the component.
// Supports ?status=disabled filter.
// GET /api/v1/infrastructure/{id}/traefik/routers
func (h *InfraComponentHandler) GetTraefikRouters(w http.ResponseWriter, r *http.Request) {
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
	statusFilter := r.URL.Query().Get("status")
	routes, err := h.store.DiscoveredRoutes.ListDiscoveredRoutesByStatus(r.Context(), id, statusFilter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  routes,
		"total": len(routes),
	})
}

// GetTraefikServices returns all traefik_services for the component.
// Supports ?status=down filter (servers_down > 0).
// GET /api/v1/infrastructure/{id}/traefik/services
func (h *InfraComponentHandler) GetTraefikServices(w http.ResponseWriter, r *http.Request) {
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
	statusFilter := r.URL.Query().Get("status")
	svcs, err := h.store.TraefikServices.ListByComponent(r.Context(), id, statusFilter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  svcs,
		"total": len(svcs),
	})
}
