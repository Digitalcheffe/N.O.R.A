package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

// DockerDiscoveryHandler serves the container discovery endpoints.
type DockerDiscoveryHandler struct {
	store *repo.Store
}

// NewDockerDiscoveryHandler returns a DockerDiscoveryHandler wired to store.
func NewDockerDiscoveryHandler(store *repo.Store) *DockerDiscoveryHandler {
	return &DockerDiscoveryHandler{store: store}
}

// Routes registers the docker discovery endpoints on r.
func (h *DockerDiscoveryHandler) Routes(r chi.Router) {
	r.Get("/docker-engines/{id}/containers", h.ListContainers)
}

// discoveredContainerResponse is the per-container shape returned by the API.
type discoveredContainerResponse struct {
	ID                   string    `json:"id"`
	ContainerName        string    `json:"container_name"`
	Image                string    `json:"image"`
	Status               string    `json:"status"`
	AppID                *string   `json:"app_id"`
	ProfileSuggestion    *string   `json:"profile_suggestion"`
	SuggestionConfidence *int      `json:"suggestion_confidence"`
	CPUPercent           *float64  `json:"cpu_percent"`
	MemPercent           *float64  `json:"mem_percent"`
	LastSeenAt           time.Time `json:"last_seen_at"`
}

type listDiscoveredContainersResponse struct {
	Data  []discoveredContainerResponse `json:"data"`
	Total int                           `json:"total"`
}

// ListContainers returns all discovered containers for a docker engine.
// GET /api/v1/docker-engines/{id}/containers
func (h *DockerDiscoveryHandler) ListContainers(w http.ResponseWriter, r *http.Request) {
	engineID := chi.URLParam(r, "id")

	// Verify the engine exists.
	if _, err := h.store.DockerEngines.Get(r.Context(), engineID); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "docker engine not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	containers, err := h.store.DiscoveredContainers.ListDiscoveredContainers(r.Context(), engineID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Collect container IDs for a batched resource reading lookup.
	containerIDs := make([]string, len(containers))
	for i, c := range containers {
		containerIDs[i] = c.ContainerID
	}

	metrics, err := h.store.Resources.LatestMetrics(r.Context(), "docker_container", containerIDs)
	if err != nil {
		// Non-fatal — return containers without metrics rather than erroring.
		metrics = map[string]map[string]float64{}
	}

	out := make([]discoveredContainerResponse, len(containers))
	for i, c := range containers {
		item := discoveredContainerResponse{
			ID:                   c.ID,
			ContainerName:        c.ContainerName,
			Image:                c.Image,
			Status:               c.Status,
			AppID:                c.AppID,
			ProfileSuggestion:    c.ProfileSuggestion,
			SuggestionConfidence: c.SuggestionConfidence,
			LastSeenAt:           c.LastSeenAt,
		}

		if m, ok := metrics[c.ContainerID]; ok {
			if v, ok := m["cpu_percent"]; ok {
				item.CPUPercent = &v
			}
			if v, ok := m["mem_percent"]; ok {
				item.MemPercent = &v
			}
		}

		out[i] = item
	}

	writeJSON(w, http.StatusOK, listDiscoveredContainersResponse{Data: out, Total: len(out)})
}
