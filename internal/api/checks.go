package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/monitor"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// syncer is the minimal interface ChecksHandler needs from the scheduler —
// just enough to trigger an immediate re-sync when a check is disabled.
type syncer interface {
	TriggerSync()
}

// ChecksHandler holds dependencies for the monitor checks resource handlers.
type ChecksHandler struct {
	checks    repo.CheckRepo
	events    repo.EventRepo
	scheduler syncer // may be nil; used to push pause/resume immediately to the runner
}

// NewChecksHandler creates a ChecksHandler with the given repositories.
// Pass a non-nil scheduler so that pausing or resuming a check takes effect
// immediately instead of waiting for the next 5-minute scheduler poll.
func NewChecksHandler(checks repo.CheckRepo, events repo.EventRepo, scheduler syncer) *ChecksHandler {
	return &ChecksHandler{checks: checks, events: events, scheduler: scheduler}
}

// Routes registers all check endpoints on r.
func (h *ChecksHandler) Routes(r chi.Router) {
	r.Get("/checks", h.List)
	r.Post("/checks", h.Create)
	r.Get("/checks/{id}", h.Get)
	r.Put("/checks/{id}", h.Update)
	r.Delete("/checks/{id}", h.Delete)
	r.Post("/checks/{id}/run", h.Run)
	r.Post("/checks/{id}/reset-baseline", h.ResetBaseline)
	r.Get("/checks/{id}/events", h.ListEvents)
}

// --- request / response types ---

type checkRequest struct {
	Name             string  `json:"name"`
	Type             string  `json:"type"`
	Target           string  `json:"target"`
	IntervalSecs     int     `json:"interval_secs"`
	AppID            *string `json:"app_id"` // nil=no change, ""=clear, "uuid"=set
	ExpectedStatus   int     `json:"expected_status"`
	SSLWarnDays      int     `json:"ssl_warn_days"`
	SSLCritDays      int     `json:"ssl_crit_days"`
	Enabled          *bool   `json:"enabled"`          // nil = no change, false = disable, true = enable
	SkipTLSVerify    *bool   `json:"skip_tls_verify"`  // nil = no change; for url checks only
	DNSRecordType    string  `json:"dns_record_type"`    // A | AAAA | MX | CNAME | TXT
	DNSExpectedValue string  `json:"dns_expected_value"` // optional substring match
	DNSResolver      string  `json:"dns_resolver"`       // optional custom resolver e.g. "8.8.8.8"
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

var validCheckTypes = map[string]bool{"ping": true, "url": true, "ssl": true, "dns": true}

func validateCheck(req checkRequest) string {
	if req.Name == "" {
		return "name is required"
	}
	if !validCheckTypes[req.Type] {
		return "type must be one of: ping, url, ssl, dns"
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
	if req.Type == "dns" {
		validRecordTypes := map[string]bool{"A": true, "AAAA": true, "MX": true, "CNAME": true, "TXT": true, "": true}
		if !validRecordTypes[strings.ToUpper(req.DNSRecordType)] {
			return "dns_record_type must be one of: A, AAAA, MX, CNAME, TXT"
		}
	}
	if req.Type == "ssl" {
		if !strings.HasPrefix(req.Target, "http://") && !strings.HasPrefix(req.Target, "https://") {
			return "target must begin with http:// or https:// for ssl checks"
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

	skipTLS := req.SkipTLSVerify != nil && *req.SkipTLSVerify

	appID := ""
	if req.AppID != nil {
		appID = *req.AppID
	}
	check := &models.MonitorCheck{
		ID:               uuid.New().String(),
		AppID:            appID,
		Name:             req.Name,
		Type:             req.Type,
		Target:           req.Target,
		IntervalSecs:     req.IntervalSecs,
		ExpectedStatus:   req.ExpectedStatus,
		SSLWarnDays:      warnDays,
		SSLCritDays:      critDays,
		SkipTLSVerify:    skipTLS,
		DNSRecordType:    strings.ToUpper(req.DNSRecordType),
		DNSExpectedValue: req.DNSExpectedValue,
		DNSResolver:      req.DNSResolver,
		Enabled:          true,
		CreatedAt:        time.Now().UTC(),
	}

	if err := h.checks.Create(r.Context(), check); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// For DNS checks: immediately resolve and capture the current value as the
	// baseline so the monitor knows what "good" looks like from day one.
	if check.Type == "dns" {
		dnsCtx, dnsCancel := context.WithTimeout(r.Context(), 10*time.Second)
		result := monitor.RunDNS(dnsCtx, check.Target, check.DNSRecordType, "", check.DNSResolver)
		dnsCancel()

		var det struct {
			Records []string `json:"records"`
		}
		if json.Unmarshal(result.Details, &det) == nil && len(det.Records) > 0 {
			check.DNSExpectedValue = det.Records[0]
			_ = h.checks.SetDNSBaseline(r.Context(), check.ID, check.DNSExpectedValue)
			now := time.Now().UTC()
			detailsStr := string(result.Details)
			_ = h.checks.UpdateStatus(r.Context(), check.ID, result.Status, detailsStr, now)
			// Re-fetch so the response includes the captured baseline and status.
			if updated, getErr := h.checks.Get(r.Context(), check.ID); getErr == nil {
				check = updated
			}
		}
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
	if req.AppID != nil {
		existing.AppID = *req.AppID
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
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.SkipTLSVerify != nil {
		existing.SkipTLSVerify = *req.SkipTLSVerify
	}
	if req.DNSRecordType != "" {
		existing.DNSRecordType = strings.ToUpper(req.DNSRecordType)
	}
	if req.DNSExpectedValue != "" {
		existing.DNSExpectedValue = req.DNSExpectedValue
	}
	if req.DNSResolver != "" {
		existing.DNSResolver = req.DNSResolver
	}

	// Re-validate only when core fields were part of the request — a
	// pause/resume or flag-only update should not be rejected because the
	// stored target happens to be an unresolved template placeholder.
	coreFieldsChanged := req.Name != "" || req.Type != "" || req.Target != "" || req.IntervalSecs != 0
	if coreFieldsChanged {
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
	}

	if err := h.checks.Update(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// When the enabled flag changed, nudge the scheduler immediately so the
	// goroutine for a paused check is cancelled (or started for a resumed one)
	// without waiting for the next 5-minute periodic sync.
	if req.Enabled != nil && h.scheduler != nil {
		h.scheduler.TriggerSync()
	}

	writeJSON(w, http.StatusOK, existing)
}

// Delete removes a check: DELETE /api/v1/checks/{id}
// Returns 409 if the check is owned by a Traefik component (delete the component instead).
func (h *ChecksHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
	if check.SourceComponentID != nil && *check.SourceComponentID != "" {
		writeError(w, http.StatusConflict, "check is managed by a Traefik component and cannot be deleted directly")
		return
	}

	if err := h.checks.Delete(r.Context(), id); err != nil {
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

	result, runErr := monitor.Run(ctx, check.Type, check.Target, check.ExpectedStatus, check.SSLWarnDays, check.SSLCritDays, check.SkipTLSVerify, check.DNSRecordType, check.DNSExpectedValue, check.DNSResolver)
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

	// Create an event on status change (always — app_id is optional).
	if result.Status != prevStatus {
		h.createStatusEvent(r.Context(), check, result)
	}

	writeJSON(w, http.StatusOK, runCheckResponse{
		Status:    result.Status,
		Result:    result.Details,
		CheckedAt: result.CheckedAt,
	})
}

// ListEvents returns recent status-change events for a specific check: GET /api/v1/checks/{id}/events
func (h *ChecksHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.checks.Get(r.Context(), id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeError(w, http.StatusNotFound, "check not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	events, _, err := h.events.List(r.Context(), repo.ListFilter{
		SourceID:   id,
		SourceType: "monitor_check",
		Limit:      50,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  events,
		"total": len(events),
	})
}

// createStatusEvent records a monitor check status change as an event.
func (h *ChecksHandler) createStatusEvent(ctx context.Context, check *models.MonitorCheck, result monitor.Result) {
	level := "info"
	if result.Status == "down" || result.Status == "critical" {
		level = "error"
	} else if result.Status == "warn" {
		level = "warn"
	}

	event := &models.Event{
		ID:         uuid.New().String(),
		Level:      level,
		SourceName: check.Name,
		SourceType: "monitor_check",
		SourceID:   check.ID,
		Title:      check.Name + " — " + result.Status,
		Payload:    string(result.Details),
		CreatedAt:  result.CheckedAt,
	}
	// Log failure but don't fail the response.
	if err := h.events.Create(ctx, event); err != nil {
		log.Printf("createStatusEvent: failed to write event for check %s: %v", check.ID, err)
	}
}

// ResetBaseline re-resolves a DNS check immediately and stores the result as
// the new baseline. Event history is not affected. Returns the updated check.
// POST /api/v1/checks/{id}/reset-baseline
func (h *ChecksHandler) ResetBaseline(w http.ResponseWriter, r *http.Request) {
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
	if check.Type != "dns" {
		writeError(w, http.StatusBadRequest, "reset-baseline is only valid for DNS checks")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	result := monitor.RunDNS(ctx, check.Target, check.DNSRecordType, "", check.DNSResolver)
	cancel()

	var det struct {
		Records []string `json:"records"`
	}
	if json.Unmarshal(result.Details, &det) != nil || len(det.Records) == 0 {
		writeError(w, http.StatusBadGateway, "DNS resolution failed — could not capture a baseline")
		return
	}

	baseline := det.Records[0]
	if err := h.checks.SetDNSBaseline(r.Context(), check.ID, baseline); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	now := time.Now().UTC()
	_ = h.checks.UpdateStatus(r.Context(), check.ID, result.Status, string(result.Details), now)

	updated, err := h.checks.Get(r.Context(), check.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
