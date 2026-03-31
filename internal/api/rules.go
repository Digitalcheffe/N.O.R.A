package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/internal/rules"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// RulesHandler handles CRUD for alert rules plus the /sources helper endpoint.
type RulesHandler struct {
	store  *repo.Store
	engine *rules.Engine
}

// NewRulesHandler creates a RulesHandler.
func NewRulesHandler(store *repo.Store, engine *rules.Engine) *RulesHandler {
	return &RulesHandler{store: store, engine: engine}
}

// Routes registers all rule endpoints on r.
// /rules/sources MUST be registered before /rules/{id} to avoid "sources"
// being treated as an ID parameter by chi.
func (h *RulesHandler) Routes(r chi.Router) {
	r.Get("/rules/sources", h.Sources)
	r.Get("/rules", h.List)
	r.Post("/rules", h.Create)
	r.Get("/rules/{id}", h.Get)
	r.Put("/rules/{id}", h.Update)
	r.Delete("/rules/{id}", h.Delete)
	r.Patch("/rules/{id}/toggle", h.Toggle)
}

// ── request / response types ──────────────────────────────────────────────────

type ruleRequest struct {
	Name            string                 `json:"name"`
	Enabled         bool                   `json:"enabled"`
	SourceID        *string                `json:"source_id"`
	SourceType      *string                `json:"source_type"`
	Severity        *string                `json:"severity"`
	Conditions      []models.RuleCondition `json:"conditions"`
	ConditionLogic  string                 `json:"condition_logic"`
	DeliveryEmail   bool                   `json:"delivery_email"`
	DeliveryPush    bool                   `json:"delivery_push"`
	DeliveryWebhook bool                   `json:"delivery_webhook"`
	WebhookURL      *string                `json:"webhook_url"`
	NotifTitle      string                 `json:"notif_title"`
	NotifBody       string                 `json:"notif_body"`
}

type sourceItem struct {
	ID    *string `json:"id"`
	Label string  `json:"label"`
	Type  *string `json:"type"`
}

type sourcesResponse struct {
	Sources []sourceItem `json:"sources"`
}

// conditionsJSON encodes a []RuleCondition to a JSON string.
func conditionsJSON(cs []models.RuleCondition) string {
	if len(cs) == 0 {
		return "[]"
	}
	b, err := json.Marshal(cs)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// conditionsSlice decodes a JSON string to []RuleCondition for the response.
func conditionsSlice(raw string) []models.RuleCondition {
	if raw == "" || raw == "[]" {
		return []models.RuleCondition{}
	}
	var cs []models.RuleCondition
	if err := json.Unmarshal([]byte(raw), &cs); err != nil {
		return []models.RuleCondition{}
	}
	return cs
}

// ruleToResponse converts a Rule model for the API response, expanding
// the Conditions JSON string into a proper array.
type ruleResponse struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	Enabled         bool                   `json:"enabled"`
	SourceID        *string                `json:"source_id"`
	SourceType      *string                `json:"source_type"`
	Severity        *string                `json:"severity"`
	Conditions      []models.RuleCondition `json:"conditions"`
	ConditionLogic  string                 `json:"condition_logic"`
	DeliveryEmail   bool                   `json:"delivery_email"`
	DeliveryPush    bool                   `json:"delivery_push"`
	DeliveryWebhook bool                   `json:"delivery_webhook"`
	WebhookURL      *string                `json:"webhook_url"`
	NotifTitle      string                 `json:"notif_title"`
	NotifBody       string                 `json:"notif_body"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

func toRuleResponse(r models.Rule) ruleResponse {
	return ruleResponse{
		ID:              r.ID,
		Name:            r.Name,
		Enabled:         r.Enabled,
		SourceID:        r.SourceID,
		SourceType:      r.SourceType,
		Severity:        r.Severity,
		Conditions:      conditionsSlice(r.Conditions),
		ConditionLogic:  r.ConditionLogic,
		DeliveryEmail:   r.DeliveryEmail,
		DeliveryPush:    r.DeliveryPush,
		DeliveryWebhook: r.DeliveryWebhook,
		WebhookURL:      r.WebhookURL,
		NotifTitle:      r.NotifTitle,
		NotifBody:       r.NotifBody,
		CreatedAt:       r.CreatedAt,
		UpdatedAt:       r.UpdatedAt,
	}
}

// ── handlers ──────────────────────────────────────────────────────────────────

// Sources returns the dynamic source list for the rule source dropdown.
// GET /api/v1/rules/sources
func (h *RulesHandler) Sources(w http.ResponseWriter, r *http.Request) {
	strPtr := func(s string) *string { return &s }
	sources := []sourceItem{
		{ID: nil, Label: "Any source", Type: nil},
	}

	// Configured apps.
	appList, err := h.store.Apps.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, a := range appList {
		id := a.ID
		typ := "app"
		sources = append(sources, sourceItem{
			ID:    &id,
			Label: a.Name,
			Type:  &typ,
		})
	}

	// Virtual sources — always available in NORA.
	dockerID := "docker"
	monitorID := "monitor"
	sources = append(sources,
		sourceItem{ID: strPtr(dockerID), Label: "Docker Engine", Type: strPtr("docker")},
		sourceItem{ID: strPtr(monitorID), Label: "Monitor Checks", Type: strPtr("monitor")},
	)

	writeJSON(w, http.StatusOK, sourcesResponse{Sources: sources})
}

// List returns all rules.
// GET /api/v1/rules
func (h *RulesHandler) List(w http.ResponseWriter, r *http.Request) {
	ruleList, err := h.store.Rules.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := make([]ruleResponse, len(ruleList))
	for i, rule := range ruleList {
		resp[i] = toRuleResponse(rule)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": resp, "total": len(resp)})
}

// Get returns a single rule by ID.
// GET /api/v1/rules/{id}
func (h *RulesHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rule, err := h.store.Rules.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toRuleResponse(rule))
}

// Create creates a new rule.
// POST /api/v1/rules
func (h *RulesHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req ruleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	logic := req.ConditionLogic
	if logic != "AND" && logic != "OR" {
		logic = "AND"
	}

	now := time.Now().UTC()
	rule := models.Rule{
		ID:              uuid.NewString(),
		Name:            req.Name,
		Enabled:         req.Enabled,
		SourceID:        emptyStringToNil(req.SourceID),
		SourceType:      emptyStringToNil(req.SourceType),
		Severity:        emptyStringToNil(req.Severity),
		Conditions:      conditionsJSON(req.Conditions),
		ConditionLogic:  logic,
		DeliveryEmail:   req.DeliveryEmail,
		DeliveryPush:    req.DeliveryPush,
		DeliveryWebhook: req.DeliveryWebhook,
		WebhookURL:      emptyStringToNil(req.WebhookURL),
		NotifTitle:      req.NotifTitle,
		NotifBody:       req.NotifBody,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	created, err := h.store.Rules.Create(r.Context(), rule)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.engine.InvalidateCache()
	writeJSON(w, http.StatusCreated, toRuleResponse(created))
}

// Update replaces an existing rule.
// PUT /api/v1/rules/{id}
func (h *RulesHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := h.store.Rules.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var req ruleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	logic := req.ConditionLogic
	if logic != "AND" && logic != "OR" {
		logic = "AND"
	}

	existing.Name = req.Name
	existing.Enabled = req.Enabled
	existing.SourceID = emptyStringToNil(req.SourceID)
	existing.SourceType = emptyStringToNil(req.SourceType)
	existing.Severity = emptyStringToNil(req.Severity)
	existing.Conditions = conditionsJSON(req.Conditions)
	existing.ConditionLogic = logic
	existing.DeliveryEmail = req.DeliveryEmail
	existing.DeliveryPush = req.DeliveryPush
	existing.DeliveryWebhook = req.DeliveryWebhook
	existing.WebhookURL = emptyStringToNil(req.WebhookURL)
	existing.NotifTitle = req.NotifTitle
	existing.NotifBody = req.NotifBody
	existing.UpdatedAt = time.Now().UTC()

	updated, err := h.store.Rules.Update(r.Context(), existing)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.engine.InvalidateCache()
	writeJSON(w, http.StatusOK, toRuleResponse(updated))
}

// Delete removes a rule.
// DELETE /api/v1/rules/{id}
func (h *RulesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.store.Rules.Get(r.Context(), id); errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}
	if err := h.store.Rules.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.engine.InvalidateCache()
	w.WriteHeader(http.StatusNoContent)
}

// Toggle flips the enabled field and returns the updated rule.
// PATCH /api/v1/rules/{id}/toggle
func (h *RulesHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rule, err := h.store.Rules.Get(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	rule.Enabled = !rule.Enabled
	rule.UpdatedAt = time.Now().UTC()

	updated, err := h.store.Rules.Update(r.Context(), rule)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.engine.InvalidateCache()
	writeJSON(w, http.StatusOK, toRuleResponse(updated))
}

// emptyStringToNil converts a *string pointer to nil when the string is empty.
func emptyStringToNil(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}
