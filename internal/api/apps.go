package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// AppsHandler holds dependencies for the apps resource handlers.
type AppsHandler struct {
	apps repo.AppRepo
}

// NewAppsHandler creates an AppsHandler with the given repository.
func NewAppsHandler(apps repo.AppRepo) *AppsHandler {
	return &AppsHandler{apps: apps}
}

// Routes registers all app endpoints on r.
func (h *AppsHandler) Routes(r chi.Router) {
	r.Get("/apps", h.List)
	r.Post("/apps", h.Create)
	r.Get("/apps/{id}", h.Get)
	r.Put("/apps/{id}", h.Update)
	r.Delete("/apps/{id}", h.Delete)
	r.Post("/apps/{id}/token/regenerate", h.RegenerateToken)
}

// --- request / response types ---

type createAppRequest struct {
	Name      string          `json:"name"`
	ProfileID string          `json:"profile_id"`
	Config    json.RawMessage `json:"config"`
	RateLimit int             `json:"rate_limit"`
}

type listAppsResponse struct {
	Data  []models.App `json:"data"`
	Total int          `json:"total"`
}

type regenerateTokenResponse struct {
	Token string `json:"token"`
}

// --- handlers ---

// List returns all apps: GET /api/v1/apps
func (h *AppsHandler) List(w http.ResponseWriter, r *http.Request) {
	apps, err := h.apps.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if apps == nil {
		apps = []models.App{}
	}
	writeJSON(w, http.StatusOK, listAppsResponse{Data: apps, Total: len(apps)})
}

// Create creates a new app: POST /api/v1/apps
func (h *AppsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	token, err := generateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	cfg := "{}"
	if len(req.Config) > 0 {
		cfg = string(req.Config)
	}
	rateLimit := req.RateLimit
	if rateLimit <= 0 {
		rateLimit = 100
	}

	app := &models.App{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Token:     token,
		ProfileID: req.ProfileID,
		Config:    cfg,
		RateLimit: rateLimit,
		CreatedAt: time.Now().UTC(),
	}

	if err := h.apps.Create(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, app)
}

// Get returns a single app: GET /api/v1/apps/{id}
func (h *AppsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	app, err := h.apps.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, app)
}

// Update replaces an app's mutable fields: PUT /api/v1/apps/{id}
func (h *AppsHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Verify the app exists first so we can return 404 before decoding.
	existing, err := h.apps.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var req createAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.ProfileID != "" {
		existing.ProfileID = req.ProfileID
	}
	if len(req.Config) > 0 {
		existing.Config = string(req.Config)
	}
	if req.RateLimit > 0 {
		existing.RateLimit = req.RateLimit
	}

	if err := h.apps.Update(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

// Delete removes an app: DELETE /api/v1/apps/{id}
func (h *AppsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := h.apps.Delete(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RegenerateToken rotates the app's ingest token: POST /api/v1/apps/{id}/token/regenerate
func (h *AppsHandler) RegenerateToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Confirm the app exists.
	if _, err := h.apps.Get(r.Context(), id); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	token, err := generateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	if err := h.apps.UpdateToken(r.Context(), id, token); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, regenerateTokenResponse{Token: token})
}

// --- helpers ---

// generateToken returns a cryptographically random 32-byte base64url token (no padding).
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
