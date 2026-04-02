package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/digitalcheffe/nora/internal/auth"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

// TOTPHandler serves TOTP enrollment and admin management endpoints.
type TOTPHandler struct {
	users  repo.UserRepo
	secret string
}

// NewTOTPHandler creates a TOTPHandler.
func NewTOTPHandler(users repo.UserRepo, secret string) *TOTPHandler {
	return &TOTPHandler{users: users, secret: secret}
}

// Routes registers TOTP endpoints.
// Verify is registered as a public route via RegisterPublicRoutes.
func (h *TOTPHandler) Routes(r chi.Router) {
	// Authenticated user manages their own TOTP
	r.Get("/auth/totp/setup", h.Setup)
	r.Post("/auth/totp/confirm", h.Confirm)
	r.Delete("/auth/totp/self", h.DisableOwn)
	r.Put("/auth/totp/self/enable", h.EnableOwn)

	// Admin manages other users' TOTP
	r.Delete("/users/{id}/totp", h.AdminDisable)
	r.Put("/users/{id}/totp/grace", h.AdminResetGrace)
}

// RegisterPublicRoutes registers the TOTP verify endpoint (no session required —
// caller only has an mfa_token at this point).
func (h *TOTPHandler) RegisterPublicRoutes(r chi.Router) {
	r.Post("/api/v1/auth/totp/verify", h.Verify)
}

// --- request / response types ---

type totpSetupResponse struct {
	URI    string `json:"uri"`
	Secret string `json:"secret"`
}

type totpConfirmRequest struct {
	Code string `json:"code"`
}

type totpVerifyRequest struct {
	MFAToken string `json:"mfa_token"`
	Code     string `json:"code"`
}

type totpVerifyResponse struct {
	Token string `json:"token"`
	User  interface{} `json:"user"`
}

// --- handlers ---

// Setup generates a TOTP secret and returns the otpauth URI.
// The secret is stored but TOTP is not enabled until Confirm succeeds.
// GET /api/v1/auth/totp/setup
func (h *TOTPHandler) Setup(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())

	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate TOTP secret")
		return
	}

	if err := h.users.SetTOTPSecret(r.Context(), userID, secret); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store TOTP secret")
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	uri := auth.TOTPUri(secret, user.Email, "NORA")
	writeJSON(w, http.StatusOK, totpSetupResponse{URI: uri, Secret: secret})
}

// Confirm verifies a TOTP code against the pending secret and enables TOTP.
// POST /api/v1/auth/totp/confirm
func (h *TOTPHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())

	var req totpConfirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}

	secret, _, _, _, err := h.users.GetTOTPData(r.Context(), userID)
	if err != nil || secret == "" {
		writeError(w, http.StatusBadRequest, "no pending TOTP setup found — call setup first")
		return
	}

	if !auth.ValidateTOTP(secret, req.Code) {
		writeError(w, http.StatusUnauthorized, "invalid code")
		return
	}

	if err := h.users.EnableTOTP(r.Context(), userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to enable TOTP")
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// Verify completes the MFA second step: validates the mfa_token and TOTP code,
// then issues a full session JWT.
// POST /api/v1/auth/totp/verify  (public — no session cookie required)
func (h *TOTPHandler) Verify(w http.ResponseWriter, r *http.Request) {
	var req totpVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.MFAToken == "" || req.Code == "" {
		writeError(w, http.StatusBadRequest, "mfa_token and code are required")
		return
	}

	claims, err := auth.ValidateMFAToken(req.MFAToken, h.secret)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired MFA token")
		return
	}

	secret, enabled, _, _, err := h.users.GetTOTPData(r.Context(), claims.UserID)
	if err != nil || !enabled || secret == "" {
		writeError(w, http.StatusUnauthorized, "TOTP not configured for this account")
		return
	}

	if !auth.ValidateTOTP(secret, req.Code) {
		writeError(w, http.StatusUnauthorized, "invalid code")
		return
	}

	user, err := h.users.GetByID(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "user not found")
		return
	}

	token, err := auth.GenerateToken(user.ID, user.Role, h.secret)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "nora_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token": token,
		"user":  user,
	})
}

// DisableOwn lets an authenticated user disable their own TOTP (requires current code).
// DELETE /api/v1/auth/totp/self
func (h *TOTPHandler) DisableOwn(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())

	var req totpConfirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}

	secret, enabled, _, _, err := h.users.GetTOTPData(r.Context(), userID)
	if err != nil || !enabled || secret == "" {
		writeError(w, http.StatusBadRequest, "TOTP is not enabled on this account")
		return
	}

	if !auth.ValidateTOTP(secret, req.Code) {
		writeError(w, http.StatusUnauthorized, "invalid code")
		return
	}

	if err := h.users.DisableTOTP(r.Context(), userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to disable TOTP")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AdminDisable lets an admin disable TOTP for any user.
// DELETE /api/v1/users/{id}/totp
func (h *TOTPHandler) AdminDisable(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.users.DisableTOTP(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to disable TOTP")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// EnableOwn re-enables TOTP for the authenticated user using their existing secret.
// PUT /api/v1/auth/totp/self/enable
func (h *TOTPHandler) EnableOwn(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())
	secret, enabled, _, _, err := h.users.GetTOTPData(r.Context(), userID)
	if err != nil || secret == "" {
		writeError(w, http.StatusBadRequest, "no TOTP secret enrolled — use setup flow")
		return
	}
	if enabled {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := h.users.EnableTOTP(r.Context(), userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to enable TOTP")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AdminResetGrace lets an admin restore a user's grace login.
// PUT /api/v1/users/{id}/totp/grace
func (h *TOTPHandler) AdminResetGrace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.users.ResetGrace(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset grace")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
