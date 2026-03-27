package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/monitor"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ChecksHandler holds dependencies for the monitor checks resource handlers.
type ChecksHandler struct {
	checks repo.CheckRepo
	events repo.EventRepo
}

// NewChecksHandler creates a ChecksHandler with the given repositories.
func NewChecksHandler(checks repo.CheckRepo, events repo.EventRepo) *ChecksHandler {
	return &ChecksHandler{checks: checks, events: events}
}

// Routes registers all check endpoints on r.
func (h *ChecksHandler) Routes(r chi.Router) {
	r.Get("/checks", h.List)
	r.Post("/checks", h.Create)
	r.Get("/checks/{id}", h.Get)
	r.Put("/checks/{id}", h.Update)
	r.Delete("/checks/{id}", h.Delete)
	r.Post("/checks/{id}/run", h.Run)
}

// --- request / response types ---

type checkRequest struct {
	Name           string  `json:"name"`
	Type           string  `json:"type"`
	Target         string  `json:"target"`
	IntervalSecs   int     `json:"interval_secs"`
	AppID          string  `json:"app_id"`
	ExpectedStatus int     `json:"expected_status"`
	SSLWarnDays    int     `json:"ssl_warn_days"`
	SSLCritDays    int     `json:"ssl_crit_days"`
	SSLSource      *string `json:"ssl_source"`       // "traefik" | "standalone" | nil
	IntegrationID  *string `json:"integration_id"`   // required when ssl_source == "traefik"
}

type listChecksResponse struct {
	Data  []models.MonitorCheck `json:"data"`
	Total int                   `json:"total"`
}

type runCheckResponse struct {
	Status    string          `json:"status"`
	Result    json.RawMessage `json:"result"`
	CheckedAt time.Time       `json:"checked_at"`
}

// --- validation ---

var validCheckTypes = map[string]bool{"ping": true, "url": true, "ssl": true}

func validateCheck(req checkRequest) string {
	if req.Name == "" {
		return "name is required"
	}
	if !validCheckTypes[req.Type] {
		return "type must be one of: ping, url, ssl"
	}
	if req.Target == "" {
		return "target is required"
	}
	if req.IntervalSecs < 30 {
		return "interval_secs must be at least 30"
	}
	if req.Type == "url" {
		if !strings.HasPrefix(req.Target, "http://") && !strings.HasPrefix(req.Target, "https://") {
			return "target must begin with http:// or https:// for url checks"
		}
	}
	// For SSL checks, only require a URL prefix in standalone mode.
	// Traefik-mode SSL checks use a bare domain name as the target.
	if req.Type == "ssl" {
		isTraefik := req.SSLSource != nil && *req.SSLSource == "traefik"
		if !isTraefik {
			if !strings.HasPrefix(req.Target, "http://") && !strings.HasPrefix(req.Target, "https://") {
				return "target must begin with http:// or https:// for standalone ssl checks"
			}
		}
	}
	return ""
}

// --- handlers ---

// List returns all checks: GET /api/v1/checks
func (h *ChecksHandler) List(w http.ResponseWriter, r *http.Request) {
	checks, err := h.checks.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, listChecksResponse{Data: checks, Total: len(checks)})
}

// Create creates a new check: POST /api/v1/checks
func (h *ChecksHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req checkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := validateCheck(req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	warnDays := req.SSLWarnDays
	if warnDays == 0 {
		warnDays = 30
	}
	critDays := req.SSLCritDays
	if critDays == 0 {
		critDays = 7
	}

	check := &models.MonitorCheck{
		ID:             uuid.New().String(),
		AppID:          req.AppID,
		Name:           req.Name,
		Type:           req.Type,
		Target:         req.Target,
		IntervalSecs:   req.IntervalSecs,
		ExpectedStatus: req.ExpectedStatus,
		SSLWarnDays:    warnDays,
		SSLCritDays:    critDays,
		SSLSource:      req.SSLSource,
		IntegrationID:  req.IntegrationID,
		Enabled:        true,
		CreatedAt:      time.Now().UTC(),
	}

	if err := h.checks.Create(r.Context(), check); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, check)
}

// Get returns a single check: GET /api/v1/checks/{id}
func (h *ChecksHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	check, err := h.checks.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "check not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, check)
}

// Update replaces a check's mutable fields: PUT /api/v1/checks/{id}
func (h *ChecksHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	existing, err := h.checks.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "check not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var req checkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Apply provided fields.
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Type != "" {
		existing.Type = req.Type
	}
	if req.Target != "" {
		existing.Target = req.Target
	}
	if req.IntervalSecs != 0 {
		existing.IntervalSecs = req.IntervalSecs
	}
	if req.AppID != "" {
		existing.AppID = req.AppID
	}
	if req.ExpectedStatus != 0 {
		existing.ExpectedStatus = req.ExpectedStatus
	}
	if req.SSLWarnDays != 0 {
		existing.SSLWarnDays = req.SSLWarnDays
	}
	if req.SSLCritDays != 0 {
		existing.SSLCritDays = req.SSLCritDays
	}
	if req.SSLSource != nil {
		existing.SSLSource = req.SSLSource
	}
	if req.IntegrationID != nil {
		existing.IntegrationID = req.IntegrationID
	}

	// Re-validate the merged state.
	merged := checkRequest{
		Name:         existing.Name,
		Type:         existing.Type,
		Target:       existing.Target,
		IntervalSecs: existing.IntervalSecs,
	}
	if msg := validateCheck(merged); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	if err := h.checks.Update(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

// Delete removes a check: DELETE /api/v1/checks/{id}
func (h *ChecksHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := h.checks.Delete(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "check not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Run executes a check immediately and returns the result: POST /api/v1/checks/{id}/run
func (h *ChecksHandler) Run(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	check, err := h.checks.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "check not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	result, runErr := monitor.Run(ctx, check.Type, check.Target, check.ExpectedStatus, check.SSLWarnDays, check.SSLCritDays)
	if runErr != nil {
		writeError(w, http.StatusInternalServerError, runErr.Error())
		return
	}

	resultJSON, _ := json.Marshal(result.Details)

	prevStatus := check.LastStatus
	if err := h.checks.UpdateStatus(r.Context(), id, result.Status, string(resultJSON), result.CheckedAt); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Create an event if the status changed to non-up and check is associated with an app.
	if result.Status != "up" && result.Status != prevStatus && check.AppID != "" {
		h.createStatusEvent(r.Context(), check, result)
	}

	writeJSON(w, http.StatusOK, runCheckResponse{
		Status:    result.Status,
		Result:    result.Details,
		CheckedAt: result.CheckedAt,
	})
}

// createStatusEvent records a monitor check failure as an app event.
func (h *ChecksHandler) createStatusEvent(ctx context.Context, check *models.MonitorCheck, result monitor.Result) {
	severity := "error"
	if result.Status == "warn" {
		severity = "warn"
	}
	displayText := check.Name + " check status: " + result.Status

	event := &models.Event{
		ID:          uuid.New().String(),
		AppID:       check.AppID,
		ReceivedAt:  result.CheckedAt,
		Severity:    severity,
		DisplayText: displayText,
		RawPayload:  string(result.Details),
		Fields:      `{"source":"monitor","check_id":"` + check.ID + `","status":"` + result.Status + `"}`,
	}
	// Log failure but don't fail the response.
	_ = h.events.Create(ctx, event)
}
