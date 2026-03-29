package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

// IntegrationDriversHandler manages the five hardcoded integration driver configurations,
// storing credentials in the settings key-value table.
type IntegrationDriversHandler struct {
	settings repo.SettingsRepo
}

// NewIntegrationDriversHandler creates an IntegrationDriversHandler.
func NewIntegrationDriversHandler(settings repo.SettingsRepo) *IntegrationDriversHandler {
	return &IntegrationDriversHandler{settings: settings}
}

// Routes registers the integration driver endpoints.
func (h *IntegrationDriversHandler) Routes(r chi.Router) {
	r.Get("/integration-drivers", h.List)
	r.Put("/integration-drivers/{name}", h.Configure)
	r.Delete("/integration-drivers/{name}", h.Disconnect)
}

// ── types ────────────────────────────────────────────────────────────────────

type integrationDriverMeta struct {
	Name         string   `json:"name"`
	Label        string   `json:"label"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
}

type integrationDriverResponse struct {
	integrationDriverMeta
	Configured bool `json:"configured"`
}

type listDriversResponse struct {
	Data  []integrationDriverResponse `json:"data"`
	Total int                         `json:"total"`
}

// ── static metadata ──────────────────────────────────────────────────────────

var allIntegrationDrivers = []integrationDriverMeta{
	{
		Name:         "traefik",
		Label:        "Traefik",
		Description:  "SSL cert discovery and routing visibility via Traefik API.",
		Capabilities: []string{"SSL discovery", "network map node", "API polling"},
	},
	{
		Name:         "proxmox",
		Label:        "Proxmox",
		Description:  "Node and VM status plus resource metrics via Proxmox REST API.",
		Capabilities: []string{"resource metrics", "VM/CT status", "API polling"},
	},
	{
		Name:         "opnsense",
		Label:        "OPNsense",
		Description:  "Network status and availability via OPNsense API.",
		Capabilities: []string{"network status", "firmware alerts", "API polling"},
	},
	{
		Name:         "synology",
		Label:        "Synology",
		Description:  "NAS resource metrics and volume health via Synology DSM API.",
		Capabilities: []string{"resource metrics", "volume health", "API polling"},
	},
	{
		Name:         "snmp",
		Label:        "SNMP",
		Description:  "Generic host polling via SNMP v2c/v3 for devices without a dedicated API.",
		Capabilities: []string{"resource metrics", "ping baseline", "generic host support"},
	},
}

// ── settings key helpers ─────────────────────────────────────────────────────

// driverKeys returns all settings keys for the named integration.
// Returns nil for unknown integration names.
func driverKeys(name string) []string {
	switch name {
	case "traefik":
		return []string{
			"integration.traefik.api_url",
			"integration.traefik.api_token",
		}
	case "proxmox":
		return []string{
			"integration.proxmox.host_url",
			"integration.proxmox.token_id",
			"integration.proxmox.token_secret",
		}
	case "opnsense":
		return []string{
			"integration.opnsense.host_url",
			"integration.opnsense.api_key",
			"integration.opnsense.api_secret",
		}
	case "synology":
		return []string{
			"integration.synology.host_url",
			"integration.synology.username",
			"integration.synology.password",
		}
	case "snmp":
		return []string{
			"integration.snmp.version",
			"integration.snmp.community",
			"integration.snmp.username",
			"integration.snmp.auth_password",
			"integration.snmp.priv_password",
		}
	default:
		return nil
	}
}

// isConfigured returns true if the integration has its primary credential stored and non-empty.
func (h *IntegrationDriversHandler) isConfigured(ctx context.Context, name string) bool {
	present := func(key string) bool {
		v, err := h.settings.Get(ctx, key)
		return err == nil && v != ""
	}
	switch name {
	case "traefik":
		return present("integration.traefik.api_url")
	case "proxmox":
		return present("integration.proxmox.host_url")
	case "opnsense":
		return present("integration.opnsense.host_url")
	case "synology":
		return present("integration.synology.host_url")
	case "snmp":
		// Either community (v2c) or username (v3) counts.
		return present("integration.snmp.community") || present("integration.snmp.username")
	default:
		return false
	}
}

// ── handlers ─────────────────────────────────────────────────────────────────

// List returns all five integration drivers with their configured status.
// Credential values are never included in the response.
// GET /api/v1/integration-drivers
func (h *IntegrationDriversHandler) List(w http.ResponseWriter, r *http.Request) {
	result := make([]integrationDriverResponse, len(allIntegrationDrivers))
	for i, m := range allIntegrationDrivers {
		result[i] = integrationDriverResponse{
			integrationDriverMeta: m,
			Configured:            h.isConfigured(r.Context(), m.Name),
		}
	}
	writeJSON(w, http.StatusOK, listDriversResponse{Data: result, Total: len(result)})
}

// Configure saves credentials for a named integration to the settings table.
// PUT /api/v1/integration-drivers/{name}
func (h *IntegrationDriversHandler) Configure(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	keys := driverKeys(name)
	if keys == nil {
		writeError(w, http.StatusBadRequest, "unknown integration: "+name)
		return
	}

	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	prefix := "integration." + name + "."
	for _, key := range keys {
		field := key[len(prefix):]
		if err := h.settings.Set(r.Context(), key, body[field]); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]bool{"configured": h.isConfigured(r.Context(), name)})
}

// Disconnect clears all credentials for a named integration.
// DELETE /api/v1/integration-drivers/{name}
func (h *IntegrationDriversHandler) Disconnect(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	keys := driverKeys(name)
	if keys == nil {
		writeError(w, http.StatusBadRequest, "unknown integration: "+name)
		return
	}

	for _, key := range keys {
		if err := h.settings.Set(r.Context(), key, ""); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
