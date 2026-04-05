package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/digitalcheffe/nora/internal/auth"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

// DigestRegistryHandler handles CRUD for the digest registry.
type DigestRegistryHandler struct {
	store *repo.Store
}

// NewDigestRegistryHandler creates a DigestRegistryHandler.
func NewDigestRegistryHandler(store *repo.Store) *DigestRegistryHandler {
	return &DigestRegistryHandler{store: store}
}

// Routes registers the digest registry endpoints.
func (h *DigestRegistryHandler) Routes(r chi.Router) {
	r.Get("/digest-registry", h.List)
	r.Put("/digest-registry/{id}/active", h.SetActive)
	r.Delete("/digest-registry/{id}", h.Delete)
}

// List returns all digest registry entries (active and inactive).
// GET /api/v1/digest-registry
func (h *DigestRegistryHandler) List(w http.ResponseWriter, r *http.Request) {
	if auth.Role(r.Context()) != "admin" {
		writeError(w, http.StatusForbidden, "admin role required")
		return
	}

	entries, err := h.store.DigestRegistry.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if entries == nil {
		entries = []models.DigestRegistryEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  entries,
		"total": len(entries),
	})
}

// SetActive sets the active flag for an entry.
// PUT /api/v1/digest-registry/{id}/active
func (h *DigestRegistryHandler) SetActive(w http.ResponseWriter, r *http.Request) {
	if auth.Role(r.Context()) != "admin" {
		writeError(w, http.StatusForbidden, "admin role required")
		return
	}

	id := chi.URLParam(r, "id")
	var req struct {
		Active bool `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.store.DigestRegistry.SetActiveByID(r.Context(), id, req.Active); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusNotFound, "entry not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Delete removes an inactive entry.
// DELETE /api/v1/digest-registry/{id}
func (h *DigestRegistryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if auth.Role(r.Context()) != "admin" {
		writeError(w, http.StatusForbidden, "admin role required")
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.store.DigestRegistry.Delete(r.Context(), id); err != nil {
		if errors.Is(err, repo.ErrConflict) {
			writeError(w, http.StatusConflict, "entry is still active — deactivate before deleting")
			return
		}
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusNotFound, "entry not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
