package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

const smtpSettingsAPIKey = "smtp"

// SettingsHandler handles generic settings endpoints (currently SMTP only).
type SettingsHandler struct {
	store *repo.Store
}

// NewSettingsHandler creates a SettingsHandler.
func NewSettingsHandler(store *repo.Store) *SettingsHandler {
	return &SettingsHandler{store: store}
}

// Routes registers the settings endpoints.
func (h *SettingsHandler) Routes(r chi.Router) {
	r.Get("/settings/smtp", h.GetSMTP)
	r.Put("/settings/smtp", h.PutSMTP)
}

// GetSMTP returns the stored SMTP config: GET /api/v1/settings/smtp
func (h *SettingsHandler) GetSMTP(w http.ResponseWriter, r *http.Request) {
	var s models.SMTPSettings
	err := h.store.Settings.GetJSON(r.Context(), smtpSettingsAPIKey, &s)
	if errors.Is(err, repo.ErrNotFound) {
		s = models.SMTPSettings{Port: 587}
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Never return the stored password in plaintext — mask it.
	if s.Pass != "" {
		s.Pass = "••••••••"
	}
	writeJSON(w, http.StatusOK, s)
}

// PutSMTP saves SMTP config: PUT /api/v1/settings/smtp
func (h *SettingsHandler) PutSMTP(w http.ResponseWriter, r *http.Request) {
	var req models.SMTPSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// If password is the mask sentinel, preserve the existing value.
	if req.Pass == "••••••••" {
		var existing models.SMTPSettings
		if err := h.store.Settings.GetJSON(r.Context(), smtpSettingsAPIKey, &existing); err == nil {
			req.Pass = existing.Pass
		}
	}

	if err := h.store.Settings.SetJSON(r.Context(), smtpSettingsAPIKey, req); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := req
	if resp.Pass != "" {
		resp.Pass = "••••••••"
	}
	writeJSON(w, http.StatusOK, resp)
}
