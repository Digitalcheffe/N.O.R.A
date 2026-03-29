package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/digitalcheffe/nora/internal/jobs"
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
	r.Post("/settings/smtp/test", h.TestSMTP)
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

// TestSMTP sends a test email using the stored SMTP config: POST /api/v1/settings/smtp/test
func (h *SettingsHandler) TestSMTP(w http.ResponseWriter, r *http.Request) {
	var s models.SMTPSettings
	err := h.store.Settings.GetJSON(r.Context(), smtpSettingsAPIKey, &s)
	if errors.Is(err, repo.ErrNotFound) || s.Host == "" {
		writeError(w, http.StatusBadRequest, "SMTP is not configured")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	to := s.To
	if to == "" {
		to = s.From
	}
	if to == "" {
		writeError(w, http.StatusBadRequest, "SMTP 'to' address is not configured")
		return
	}

	if err := jobs.SendMail(
		s.Host, s.Port, s.User, s.Pass, s.From,
		[]string{to},
		"NORA SMTP Test",
		"<p>This is a test email from NORA. If you received this, your SMTP configuration is working correctly.</p>",
	); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "sent", "to": to})
}
