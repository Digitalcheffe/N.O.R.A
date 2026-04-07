package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/icons"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/internal/scanner/discovery"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// AppsHandler holds dependencies for the apps resource handlers.
type AppsHandler struct {
	apps               repo.AppRepo
	checks             repo.CheckRepo             // may be nil
	iconsFetcher       *icons.Fetcher             // may be nil
	profiler           apptemplate.Loader         // may be nil; used to resolve icon slug overrides
	appMetricSnapshots repo.AppMetricSnapshotRepo // may be nil
	store              *repo.Store                // may be nil; used for on-demand polling
}

// NewAppsHandler creates an AppsHandler with the given repository.
func NewAppsHandler(apps repo.AppRepo, fetcher *icons.Fetcher, checks repo.CheckRepo, profiler apptemplate.Loader, appMetricSnapshots repo.AppMetricSnapshotRepo, store *repo.Store) *AppsHandler {
	return &AppsHandler{apps: apps, iconsFetcher: fetcher, checks: checks, profiler: profiler, appMetricSnapshots: appMetricSnapshots, store: store}
}

// Routes registers all app endpoints on r.
func (h *AppsHandler) Routes(r chi.Router) {
	r.Get("/apps", h.List)
	r.Post("/apps", h.Create)
	r.Get("/apps/{id}", h.Get)
	r.Put("/apps/{id}", h.Update)
	r.Delete("/apps/{id}", h.Delete)
	r.Post("/apps/{id}/token/regenerate", h.RegenerateToken)
	r.Get("/apps/{id}/metrics", h.GetMetrics)
	r.Post("/apps/{id}/poll", h.PollNow)
	r.Get("/apps/{id}/chain", h.GetChain)
}

// PollNow triggers an immediate API poll for a single app: POST /apps/{id}/poll.
func (h *AppsHandler) PollNow(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.store == nil || h.profiler == nil {
		http.Error(w, "polling not available", http.StatusServiceUnavailable)
		return
	}
	if err := discovery.PollOneApp(r.Context(), h.store, h.profiler, id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Infrastructure chain ──────────────────────────────────────────────────────

type chainNode struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`  // IP for infra components, short image for containers
	IconURL string `json:"icon_url,omitempty"` // profile icon URL for app/container nodes
}

type chainTraefikRoute struct {
	Router  string `json:"router"`
	Rule    string `json:"rule"`
	Service string `json:"service"`
	Status  string `json:"status"`
}

type getChainResponse struct {
	Chain   []chainNode         `json:"chain"`
	Traefik []chainTraefikRoute `json:"traefik"`
}

// GetChain handles GET /apps/{id}/chain.
// Walks component_links upward from the app and resolves display names + statuses.
// Also returns all Traefik routes linked to the app.
func (h *AppsHandler) GetChain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	if h.store == nil {
		writeJSON(w, http.StatusOK, getChainResponse{Chain: []chainNode{}, Traefik: []chainTraefikRoute{}})
		return
	}

	app, err := h.apps.Get(ctx, id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// App node — status comes from the caller (appSummary) so we leave it for
	// the frontend to enrich; return empty string and let the UI substitute.
	appIconURL := ""
	if app.ProfileID != "" {
		appIconURL = "/api/v1/icons/" + app.ProfileID
	}
	chain := []chainNode{{Type: "app", ID: app.ID, Name: app.Name, Status: "", IconURL: appIconURL}}

	// Walk component_links upward, capped at 10 hops to prevent cycles.
	curType, curID := "app", id
	for i := 0; i < 10; i++ {
		link, err := h.store.ComponentLinks.GetParent(ctx, curType, curID)
		if errors.Is(err, repo.ErrNotFound) || link == nil {
			break
		}
		if err != nil {
			break // non-fatal; return what we have
		}
		node := h.resolveChainNode(ctx, link.ParentType, link.ParentID)
		chain = append(chain, node)
		curType, curID = link.ParentType, link.ParentID
	}

	// Traefik routes linked to this app.
	routes, _ := h.store.DiscoveredRoutes.ListByAppID(ctx, id)
	traefik := make([]chainTraefikRoute, 0, len(routes))
	for _, ro := range routes {
		svc := ""
		if ro.ServiceName != nil {
			svc = *ro.ServiceName
		}
		traefik = append(traefik, chainTraefikRoute{
			Router:  ro.RouterName,
			Rule:    ro.Rule,
			Service: svc,
			Status:  ro.RouterStatus,
		})
	}

	writeJSON(w, http.StatusOK, getChainResponse{Chain: chain, Traefik: traefik})
}

// resolveChainNode fetches the display name, status, detail, and icon for a component_links parent node.
func (h *AppsHandler) resolveChainNode(ctx context.Context, nodeType, nodeID string) chainNode {
	if nodeType == "container" {
		if c, err := h.store.DiscoveredContainers.GetDiscoveredContainer(ctx, nodeID); err == nil {
			iconURL := ""
			if c.AppID != nil && *c.AppID != "" {
				// Use the linked app's profile icon for the container node.
				if linkedApp, err := h.store.Apps.Get(ctx, *c.AppID); err == nil && linkedApp.ProfileID != "" {
					iconURL = "/api/v1/icons/" + linkedApp.ProfileID
				}
			}
			return chainNode{Type: nodeType, ID: nodeID, Name: c.ContainerName, Status: c.Status, Detail: shortImage(c.Image), IconURL: iconURL}
		}
	} else {
		if ic, err := h.store.InfraComponents.Get(ctx, nodeID); err == nil {
			return chainNode{Type: nodeType, ID: nodeID, Name: ic.Name, Status: ic.LastStatus, Detail: ic.IP}
		}
	}
	return chainNode{Type: nodeType, ID: nodeID, Name: nodeID, Status: ""}
}

// shortImage strips the registry prefix and tag from a container image string,
// returning just the image name. E.g. "ghcr.io/linuxserver/sonarr:latest" → "sonarr".
func shortImage(image string) string {
	// Strip registry (anything before the last '/')
	if i := len(image) - 1; i > 0 {
		for j := len(image) - 1; j >= 0; j-- {
			if image[j] == '/' {
				image = image[j+1:]
				break
			}
		}
	}
	// Strip tag
	if i := len(image) - 1; i > 0 {
		for j := 0; j < len(image); j++ {
			if image[j] == ':' {
				image = image[:j]
				break
			}
		}
	}
	return image
}

// iconSlugForProfile returns the CDN icon slug override for a profileID by
// looking it up in the app template. Returns "" if no override or no profiler.
func (h *AppsHandler) iconSlugForProfile(profileID string) string {
	if h.profiler == nil || profileID == "" {
		return ""
	}
	t, err := h.profiler.Get(profileID)
	if err != nil || t == nil {
		return ""
	}
	return t.Meta.Icon
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

type listMetricsResponse struct {
	Data  []models.AppMetricSnapshot `json:"data"`
	Total int                        `json:"total"`
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
		h.iconsFetcher.EnsureIcon(app.ProfileID, h.iconSlugForProfile(app.ProfileID))
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
		h.iconsFetcher.EnsureIcon(existing.ProfileID, h.iconSlugForProfile(existing.ProfileID))
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

// GetMetrics returns all metric snapshots for an app: GET /api/v1/apps/{id}/metrics
func (h *AppsHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.apps.Get(r.Context(), id); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.appMetricSnapshots == nil {
		writeJSON(w, http.StatusOK, listMetricsResponse{Data: []models.AppMetricSnapshot{}, Total: 0})
		return
	}
	snapshots, err := h.appMetricSnapshots.ListByApp(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, listMetricsResponse{Data: snapshots, Total: len(snapshots)})
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
