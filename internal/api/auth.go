package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/digitalcheffe/nora/internal/auth"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler serves authentication endpoints.
type AuthHandler struct {
	users    repo.UserRepo
	settings repo.SettingsRepo
	secret   string
}

// NewAuthHandler creates an AuthHandler.
func NewAuthHandler(users repo.UserRepo, settings repo.SettingsRepo, secret string) *AuthHandler {
	return &AuthHandler{users: users, settings: settings, secret: secret}
}

// Routes registers auth endpoints on r.
// Public routes (login, setup-required, first-run register) must be registered
// outside the protected group; call RegisterPublicRoutes for those.
func (h *AuthHandler) Routes(r chi.Router) {
	r.Post("/auth/logout", h.Logout)
	r.Get("/auth/me", h.Me)
}

// RegisterPublicRoutes registers auth endpoints that do not require a session.
func (h *AuthHandler) RegisterPublicRoutes(r chi.Router) {
	r.Get("/api/v1/auth/setup-required", h.SetupRequired)
	r.Post("/api/v1/auth/login", h.Login)
	// Register is conditionally public: open for first user, admin-only thereafter.
	// We handle the guard inside the handler itself.
	r.Post("/api/v1/auth/register", h.Register)
}

// --- request / response types ---

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string      `json:"token"`
	User  models.User `json:"user"`
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// --- handlers ---

// SetupRequired reports whether the first-run setup flow is needed.
// GET /api/v1/auth/setup-required
func (h *AuthHandler) SetupRequired(w http.ResponseWriter, r *http.Request) {
	n, err := h.users.Count(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"required": n == 0})
}

// Register creates a new user.
// POST /api/v1/auth/register
// Open (no auth) when the users table is empty; requires admin JWT otherwise.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	n, err := h.users.Count(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// After first user exists, require admin auth.
	if n > 0 {
		token := extractBearerOrCookie(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		claims, err := auth.ValidateToken(token, h.secret)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if claims.Role != "admin" {
			writeError(w, http.StatusForbidden, "admin role required")
			return
		}
	}

	var req registerRequest
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
	if n == 0 {
		// First user is always admin regardless of the role field.
		role = "admin"
	} else if role == "" {
		role = "member"
	}
	if role != "admin" && role != "member" {
		writeError(w, http.StatusBadRequest, "role must be admin or member")
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	u := &models.User{
		ID:    uuid.NewString(),
		Email: req.Email,
		Role:  role,
	}
	if err := h.users.Create(r.Context(), u, string(hashed)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	created, err := h.users.GetByID(r.Context(), u.ID)
	if err != nil {
		writeJSON(w, http.StatusCreated, u)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// Login validates credentials and returns a JWT in the response body and as a cookie.
// POST /api/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, hash, err := h.users.GetByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// --- MFA gate ---
	_, totpEnabled, totpGrace, totpExempt, _ := h.users.GetTOTPData(r.Context(), user.ID)
	mfaRequired := loadMFARequired(r.Context(), h.settings)

	if totpEnabled {
		// User has TOTP set up — require the second step.
		mfaToken, err := auth.GenerateMFAToken(user.ID, h.secret)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate MFA token")
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"mfa_required": true,
			"mfa_token":    mfaToken,
		})
		return
	}

	if mfaRequired && !totpExempt {
		if !totpGrace {
			// Grace used up — block login until admin resets or user enrolls.
			writeError(w, http.StatusForbidden, "MFA enrollment required. Ask an admin to reset your grace login.")
			return
		}
		// Use the one grace login — clear it so the next login is blocked.
		_ = h.users.ClearGrace(r.Context(), user.ID)
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
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	resp := map[string]interface{}{
		"token": token,
		"user":  user,
	}
	// Flag to the frontend that TOTP enrollment is required on next login.
	if mfaRequired && !totpExempt && !totpEnabled {
		resp["mfa_enrollment_required"] = true
	}
	// Flag if the user's current password doesn't meet the active policy.
	policy := loadPasswordPolicy(r.Context(), h.settings)
	if err := validatePassword(req.Password, policy); err != nil {
		resp["pw_policy_noncompliant"] = true
	}
	writeJSON(w, http.StatusOK, resp)
}

// Logout clears the session cookie.
// POST /api/v1/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "nora_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

// Me returns the currently authenticated user.
// GET /api/v1/auth/me
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserID(r.Context())
	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// extractBearerOrCookie reads a token from Authorization header or cookie.
func extractBearerOrCookie(r *http.Request) string {
	if h := r.Header.Get("Authorization"); len(h) > 7 && h[:7] == "Bearer " {
		return h[7:]
	}
	if c, err := r.Cookie("nora_session"); err == nil {
		return c.Value
	}
	return ""
}
