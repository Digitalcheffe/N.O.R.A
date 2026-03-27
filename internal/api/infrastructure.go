package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// InfraHandler holds dependencies for the infrastructure integrations resource.
type InfraHandler struct {
	infraRepo  repo.InfraRepo
	syncWorker *infra.SyncWorker
}

// NewInfraHandler returns an InfraHandler wired to infraRepo and syncWorker.
func NewInfraHandler(infraRepo repo.InfraRepo, syncWorker *infra.SyncWorker) *InfraHandler {
	return &InfraHandler{infraRepo: infraRepo, syncWorker: syncWorker}
}

// Routes registers all integration endpoints on r.
func (h *InfraHandler) Routes(r chi.Router) {
	r.Get("/integrations", h.List)
	r.Post("/integrations", h.Create)
	r.Get("/integrations/{id}", h.Get)
	r.Put("/integrations/{id}", h.Update)
	r.Delete("/integrations/{id}", h.Delete)
	r.Post("/integrations/{id}/sync", h.Sync)
	r.Get("/integrations/{id}/certs", h.ListCerts)
}

// ── request / response types ─────────────────────────────────────────────────

type integrationRequest struct {
	Type   string  `json:"type"`
	Name   string  `json:"name"`
	APIURL string  `json:"api_url"`
	APIKey *string `json:"api_key"`
}

type listIntegrationsResponse struct {
	Data  []*models.InfraIntegration `json:"data"`
	Total int                        `json:"total"`
}

type listCertsResponse struct {
	Data  []*models.TraefikCert `json:"data"`
	Total int                   `json:"total"`
}

type syncResponse struct {
	Status     string    `json:"status"`
	CertsFound int       `json:"certs_found"`
	SyncedAt   time.Time `json:"synced_at"`
}

// ── validation ───────────────────────────────────────────────────────────────

var validIntegrationTypes = map[string]bool{"traefik": true}

func validateIntegration(req integrationRequest) string {
	if req.Name == "" {
		return "name is required"
	}
	if !validIntegrationTypes[req.Type] {
		return "type must be: traefik"
	}
	if req.APIURL == "" {
		return "api_url is required"
	}
	return ""
}

// ── handlers ─────────────────────────────────────────────────────────────────

// List returns all integrations: GET /api/v1/integrations
func (h *InfraHandler) List(w http.ResponseWriter, r *http.Request) {
	integrations, err := h.infraRepo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, listIntegrationsResponse{Data: integrations, Total: len(integrations)})
}

// Create creates a new integration: POST /api/v1/integrations
func (h *InfraHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req integrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := validateIntegration(req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	integration := &models.InfraIntegration{
		ID:        uuid.New().String(),
		Type:      req.Type,
		Name:      req.Name,
		APIURL:    req.APIURL,
		APIKey:    req.APIKey,
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
	}
	if err := h.infraRepo.Create(r.Context(), integration); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, integration)
}

// Get returns a single integration: GET /api/v1/integrations/{id}
func (h *InfraHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	integration, err := h.infraRepo.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "integration not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, integration)
}

// Update replaces mutable fields on an integration: PUT /api/v1/integrations/{id}
func (h *InfraHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	existing, err := h.infraRepo.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "integration not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var req integrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.APIURL != "" {
		existing.APIURL = req.APIURL
	}
	existing.APIKey = req.APIKey

	merged := integrationRequest{Type: existing.Type, Name: existing.Name, APIURL: existing.APIURL}
	if msg := validateIntegration(merged); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	if err := h.infraRepo.Update(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

// Delete removes an integration: DELETE /api/v1/integrations/{id}
func (h *InfraHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := h.infraRepo.Delete(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "integration not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Sync triggers an immediate sync: POST /api/v1/integrations/{id}/sync
func (h *InfraHandler) Sync(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	n, err := h.syncWorker.SyncOne(ctx, id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "integration not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, syncResponse{
		Status:     "ok",
		CertsFound: n,
		SyncedAt:   time.Now().UTC(),
	})
}

// ListCerts returns cached certs for an integration: GET /api/v1/integrations/{id}/certs
func (h *InfraHandler) ListCerts(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Verify the integration exists.
	if _, err := h.infraRepo.Get(r.Context(), id); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "integration not found")
		return
	}

	certs, err := h.infraRepo.ListCerts(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, listCertsResponse{Data: certs, Total: len(certs)})
}
