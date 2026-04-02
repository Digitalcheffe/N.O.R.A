package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/digitalcheffe/nora/internal/icons"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// AppsHandler holds dependencies for the apps resource handlers.
type AppsHandler struct {
	apps         repo.AppRepo
	checks       repo.CheckRepo  // may be nil
	iconsFetcher *icons.Fetcher  // may be nil
}

// NewAppsHandler creates an AppsHandler with the given repository.
func NewAppsHandler(apps repo.AppRepo, fetcher *icons.Fetcher, checks repo.CheckRepo) *AppsHandler {
	return &AppsHandler{apps: apps, iconsFetcher: fetcher, checks: checks}
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
	ProfileID *string         `json:"profile_id"` // pointer: null = don't change, "" = clear, "id" = set
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

	cfg := models.ConfigJSON("{}")
	if len(req.Config) > 0 {
		cfg = models.ConfigJSON(req.Config)
	}
	rateLimit := req.RateLimit
	if rateLimit <= 0 {
		rateLimit = 100
	}

	profileID := ""
	if req.ProfileID != nil {
		profileID = *req.ProfileID
	}
	app := &models.App{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Token:     token,
		ProfileID: profileID,
		Config:    cfg,
		RateLimit: rateLimit,
		CreatedAt: time.Now().UTC(),
	}

	if err := h.apps.Create(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.iconsFetcher != nil {
		h.iconsFetcher.EnsureIcon(app.ProfileID)
	}
	h.syncMonitorCheck(r.Context(), app, monitorURLFromConfig(app.Config))
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
	if req.ProfileID != nil {
		existing.ProfileID = *req.ProfileID
	}
	if len(req.Config) > 0 {
		existing.Config = models.ConfigJSON(req.Config)
	}
	if req.RateLimit > 0 {
		existing.RateLimit = req.RateLimit
	}

	if err := h.apps.Update(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.iconsFetcher != nil {
		h.iconsFetcher.EnsureIcon(existing.ProfileID)
	}
	h.syncMonitorCheck(r.Context(), existing, monitorURLFromConfig(existing.Config))
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

// syncMonitorCheck ensures a URL monitor check exists for the given app when
// monitor_url is set in its config. It creates the check on first use and
// updates the target if the URL changes. Errors are logged but not fatal.
func (h *AppsHandler) syncMonitorCheck(ctx context.Context, app *models.App, monitorURL string) {
	if h.checks == nil || monitorURL == "" {
		return
	}
	all, err := h.checks.List(ctx)
	if err != nil {
		return
	}
	// Look for an existing auto-managed URL check for this app.
	for i := range all {
		c := &all[i]
		if c.AppID == app.ID && c.Type == "url" {
			if c.Target != monitorURL {
				c.Target = monitorURL
				c.Name = app.Name + " — uptime"
				_ = h.checks.Update(ctx, c)
			}
			return
		}
	}
	// None found — create one.
	check := &models.MonitorCheck{
		ID:           uuid.New().String(),
		AppID:        app.ID,
		Name:         app.Name + " — uptime",
		Type:         "url",
		Target:       monitorURL,
		IntervalSecs: 60,
		SSLWarnDays:  30,
		SSLCritDays:  7,
		Enabled:      true,
		CreatedAt:    time.Now().UTC(),
	}
	_ = h.checks.Create(ctx, check)
}

// monitorURLFromConfig extracts the monitor_url string from app config JSON.
func monitorURLFromConfig(cfg models.ConfigJSON) string {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(cfg), &m); err != nil {
		return ""
	}
	s, _ := m["monitor_url"].(string)
	return s
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
