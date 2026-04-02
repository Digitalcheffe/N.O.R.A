package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"unicode"

	"github.com/digitalcheffe/nora/internal/jobs"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

const smtpSettingsAPIKey = "smtp"
const passwordPolicyKey = "password_policy"
const mfaRequiredKey = "mfa_required"

// loadMFARequired returns true if the global MFA-required setting is enabled.
func loadMFARequired(ctx context.Context, s repo.SettingsRepo) bool {
	var v struct {
		Required bool `json:"required"`
	}
	if err := s.GetJSON(ctx, mfaRequiredKey, &v); err != nil {
		return false
	}
	return v.Required
}

// loadPasswordPolicy fetches the active policy, falling back to defaults on miss.
func loadPasswordPolicy(ctx context.Context, s repo.SettingsRepo) models.PasswordPolicy {
	var p models.PasswordPolicy
	if err := s.GetJSON(ctx, passwordPolicyKey, &p); err != nil {
		return models.DefaultPasswordPolicy()
	}
	if p.MinLength < 1 {
		p.MinLength = models.DefaultPasswordPolicy().MinLength
	}
	return p
}

// validatePassword checks pw against policy and returns a user-facing error or nil.
func validatePassword(pw string, p models.PasswordPolicy) error {
	if len(pw) < p.MinLength {
		return fmt.Errorf("password must be at least %d characters", p.MinLength)
	}
	if p.RequireUppercase {
		ok := false
		for _, c := range pw {
			if unicode.IsUpper(c) {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("password must contain at least one uppercase letter")
		}
	}
	if p.RequireNumber {
		ok := false
		for _, c := range pw {
			if unicode.IsDigit(c) {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("password must contain at least one number")
		}
	}
	if p.RequireSpecial {
		ok := false
		for _, c := range pw {
			if !unicode.IsLetter(c) && !unicode.IsDigit(c) {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("password must contain at least one special character")
		}
	}
	return nil
}

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
	r.Get("/settings/password-policy", h.GetPasswordPolicy)
	r.Put("/settings/password-policy", h.PutPasswordPolicy)
	r.Get("/settings/mfa-required", h.GetMFARequired)
	r.Put("/settings/mfa-required", h.PutMFARequired)
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

// GetPasswordPolicy returns the current password policy: GET /api/v1/settings/password-policy
func (h *SettingsHandler) GetPasswordPolicy(w http.ResponseWriter, r *http.Request) {
	p := loadPasswordPolicy(r.Context(), h.store.Settings)
	writeJSON(w, http.StatusOK, p)
}

// PutPasswordPolicy saves the password policy: PUT /api/v1/settings/password-policy
func (h *SettingsHandler) PutPasswordPolicy(w http.ResponseWriter, r *http.Request) {
	var req models.PasswordPolicy
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.MinLength < 1 {
		req.MinLength = 8
	}
	if err := h.store.Settings.SetJSON(r.Context(), passwordPolicyKey, req); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, req)
}

// GetMFARequired returns the global MFA-required setting: GET /api/v1/settings/mfa-required
func (h *SettingsHandler) GetMFARequired(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Required bool `json:"required"`
	}
	_ = h.store.Settings.GetJSON(r.Context(), mfaRequiredKey, &v)
	writeJSON(w, http.StatusOK, v)
}

// PutMFARequired saves the global MFA-required setting: PUT /api/v1/settings/mfa-required
func (h *SettingsHandler) PutMFARequired(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Required bool `json:"required"`
	}
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.store.Settings.SetJSON(r.Context(), mfaRequiredKey, v); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
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
