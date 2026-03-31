package api

import (
	"net/http"
	"time"

	"github.com/digitalcheffe/nora/internal/jobs"
	"github.com/go-chi/chi/v5"
)

// JobsHandler handles the jobs registry API endpoints.
type JobsHandler struct {
	registry *jobs.Registry
}

// NewJobsHandler creates a JobsHandler wired to the given registry.
func NewJobsHandler(registry *jobs.Registry) *JobsHandler {
	return &JobsHandler{registry: registry}
}

// Routes registers the jobs endpoints on r.
func (h *JobsHandler) Routes(r chi.Router) {
	r.Get("/jobs", h.List)
	r.Post("/jobs/{id}/run", h.Run)
}

type jobResponse struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Category      string  `json:"category"`
	LastRunAt     *string `json:"last_run_at"`
	LastRunStatus *string `json:"last_run_status"`
}

// List returns all registered jobs: GET /api/v1/jobs
func (h *JobsHandler) List(w http.ResponseWriter, r *http.Request) {
	entries := h.registry.List()
	out := make([]jobResponse, 0, len(entries))
	for _, e := range entries {
		j := jobResponse{
			ID:            e.ID,
			Name:          e.Name,
			Description:   e.Description,
			Category:      e.Category,
			LastRunStatus: e.LastRunStatus(),
		}
		if t := e.LastRunAt(); t != nil {
			s := t.Format(time.RFC3339)
			j.LastRunAt = &s
		}
		out = append(out, j)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

// Run triggers a job immediately: POST /api/v1/jobs/{id}/run
// Runs synchronously with a 60-second timeout and returns the result.
func (h *JobsHandler) Run(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.registry.Get(id) == nil {
		writeError(w, http.StatusNotFound, "job not found: "+id)
		return
	}

	start := time.Now()
	err := h.registry.Run(r.Context(), id)
	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":      "error",
			"error":       err.Error(),
			"duration_ms": durationMs,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"duration_ms": durationMs,
	})
}
