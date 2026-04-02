package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/digitalcheffe/nora/internal/auth"
	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/jobs"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// UsersHandler serves user management endpoints.
type UsersHandler struct {
	users    repo.UserRepo
	settings repo.SettingsRepo
	cfg      *config.Config
}

// NewUsersHandler creates a UsersHandler.
func NewUsersHandler(users repo.UserRepo, settings repo.SettingsRepo, cfg *config.Config) *UsersHandler {
	return &UsersHandler{users: users, settings: settings, cfg: cfg}
}

// Routes registers user endpoints on r.
func (h *UsersHandler) Routes(r chi.Router) {
	r.Get("/users", h.List)
	r.Post("/users", h.Create)
	r.Put("/users/me/password", h.ChangePassword)
	r.Put("/users/{id}", h.Update)
	r.Delete("/users/{id}", h.Delete)
	r.Put("/users/{id}/password", h.SetPassword)
	r.Put("/users/{id}/totp/exempt", h.SetTOTPExempt)
}

// --- request / response types ---

type createUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type listUsersResponse struct {
	Data  []models.User `json:"data"`
	Total int           `json:"total"`
}

// --- handlers ---

// List returns all users: GET /api/v1/users
func (h *UsersHandler) List(w http.ResponseWriter, r *http.Request) {
	users, err := h.users.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, listUsersResponse{Data: users, Total: len(users)})
}

// Create adds a new user: POST /api/v1/users
func (h *UsersHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if req.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}
	policy := loadPasswordPolicy(r.Context(), h.settings)
	if err := validatePassword(req.Password, policy); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	role := req.Role
	if role == "" {
		role = "member"
	}
	if role != "admin" && role != "member" {
		writeError(w, http.StatusBadRequest, "role must be admin or member")
		return
	}

	u := &models.User{
		ID:    uuid.NewString(),
		Email: req.Email,
		Role:  role,
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	passwordHash := string(hashed)

	if err := h.users.Create(r.Context(), u, passwordHash); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Fetch the created user to get the DB-populated created_at.
	created, err := h.users.GetByID(r.Context(), u.ID)
	if err != nil {
		writeJSON(w, http.StatusCreated, u)
		return
	}

	// Send invite email if SMTP is configured (fire-and-forget).
	if h.cfg.SMTPHost != "" {
		go func() {
			mfaRequired := loadMFARequired(r.Context(), h.settings)
			mfaLine := ""
			if mfaRequired {
				mfaLine = `<p style="margin:12px 0;color:#f59e0b;">&#9888; Multi-factor authentication is required. Please set up TOTP under your profile after logging in.</p>`
			}
			body := fmt.Sprintf(`<!DOCTYPE html><html><body style="font-family:sans-serif;color:#e2e8f0;background:#0f172a;padding:32px;">
<h2 style="color:#fff;margin:0 0 8px;">Welcome to NORA</h2>
<p style="color:#94a3b8;margin:0 0 24px;">Your account has been created.</p>
<table style="background:#1e293b;border-radius:8px;padding:20px;width:100%%;max-width:480px;">
  <tr><td style="padding:6px 0;color:#94a3b8;font-size:13px;">Email</td><td style="padding:6px 0;font-size:13px;">%s</td></tr>
  <tr><td style="padding:6px 0;color:#94a3b8;font-size:13px;">Password</td><td style="padding:6px 0;font-size:13px;font-family:monospace;">%s</td></tr>
  <tr><td style="padding:6px 0;color:#94a3b8;font-size:13px;">Role</td><td style="padding:6px 0;font-size:13px;">%s</td></tr>
</table>
%s
<p style="margin:24px 0 0;font-size:12px;color:#475569;">Please change your password after your first login.</p>
</body></html>`, req.Email, req.Password, role, mfaLine)
			if err := jobs.SendMail(h.cfg.SMTPHost, h.cfg.SMTPPort, h.cfg.SMTPUser, h.cfg.SMTPPass, h.cfg.SMTPFrom,
				[]string{req.Email}, "Your NORA account", body); err != nil {
				log.Printf("invite email to %s: %v", req.Email, err)
			}
		}()
	}

	writeJSON(w, http.StatusCreated, created)
}

// Delete removes a user: DELETE /api/v1/users/{id}
func (h *UsersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := h.users.Delete(r.Context(), id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type setPasswordRequest struct {
	Password string `json:"password"`
}

// SetPassword lets an admin set any user's password: PUT /api/v1/users/{id}/password
func (h *UsersHandler) SetPassword(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req setPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}
	policy := loadPasswordPolicy(r.Context(), h.settings)
	if err := validatePassword(req.Password, policy); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	if err := h.users.UpdatePassword(r.Context(), id, string(hashed)); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangePassword updates the authenticated user's password.
// PUT /api/v1/users/me/password
func (h *UsersHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())

	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "current_password and new_password are required")
		return
	}
	policy := loadPasswordPolicy(r.Context(), h.settings)
	if err := validatePassword(req.NewPassword, policy); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Get user by ID to retrieve email, then look up by email for hash.
	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, hash, err := h.users.GetByEmail(r.Context(), user.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.CurrentPassword)); err != nil {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	if err := h.users.UpdatePassword(r.Context(), userID, string(newHash)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type updateUserRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// Update updates a user's email and role: PUT /api/v1/users/{id}
func (h *UsersHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if req.Role != "admin" && req.Role != "member" {
		writeError(w, http.StatusBadRequest, "role must be admin or member")
		return
	}
	if err := h.users.UpdateUser(r.Context(), id, req.Email, req.Role); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	updated, err := h.users.GetByID(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

type setTOTPExemptRequest struct {
	Exempt bool `json:"exempt"`
}

// SetTOTPExempt sets or clears the totp_exempt flag for a user: PUT /api/v1/users/{id}/totp/exempt
func (h *UsersHandler) SetTOTPExempt(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req setTOTPExemptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.users.SetTOTPExempt(r.Context(), id, req.Exempt); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
