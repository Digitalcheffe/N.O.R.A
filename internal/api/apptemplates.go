package api

import (
	"encoding/json"
	"net/http"

	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// AppTemplatesHandler serves the app template library API.
type AppTemplatesHandler struct {
	registry       *apptemplate.Registry
	customProfiles repo.CustomProfileRepo
}

// NewProfilesHandler creates an AppTemplatesHandler backed by the given registry and custom-profile repo.
func NewProfilesHandler(registry *apptemplate.Registry, customProfiles repo.CustomProfileRepo) *AppTemplatesHandler {
	return &AppTemplatesHandler{registry: registry, customProfiles: customProfiles}
}

// Routes registers app template endpoints on r.
func (h *AppTemplatesHandler) Routes(r chi.Router) {
	r.Get("/app-templates", h.List)
	r.Get("/app-templates/{id}", h.Get)
	r.Post("/app-templates/validate", h.Validate)
	r.Post("/app-templates/custom", h.CreateCustom)
	r.Get("/app-templates/custom", h.ListCustom)
}

// appTemplateMeta is the list-response shape — meta fields only, no internals.
type appTemplateMeta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Logo        string `json:"logo"`
	Description string `json:"description"`
	Capability  string `json:"capability"`
}

// appTemplateDetail is the full response shape returned by GET /app-templates/{id}.
type appTemplateDetail struct {
	ID          string                    `json:"id"`
	AppTemplate *apptemplate.AppTemplate  `json:"app_template"`
}

// List handles GET /app-templates — returns meta for all registered app templates.
func (h *AppTemplatesHandler) List(w http.ResponseWriter, r *http.Request) {
	all := h.registry.List()

	items := make([]appTemplateMeta, 0, len(all))
	for id, t := range all {
		items = append(items, appTemplateMeta{
			ID:          id,
			Name:        t.Meta.Name,
			Category:    t.Meta.Category,
			Logo:        t.Meta.Logo,
			Description: t.Meta.Description,
			Capability:  t.Meta.Capability,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  items,
		"total": len(items),
	})
}

// Get handles GET /app-templates/{id} — returns the full template including setup instructions.
func (h *AppTemplatesHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	t, err := h.registry.Get(id)
	if err != nil || t == nil {
		writeError(w, http.StatusNotFound, "app template not found")
		return
	}

	writeJSON(w, http.StatusOK, appTemplateDetail{
		ID:          id,
		AppTemplate: t,
	})
}

// validateRequest is the body for POST /app-templates/validate.
type validateRequest struct {
	YAML string `json:"yaml"`
}

// validateResponse is returned by POST /app-templates/validate.
type validateResponse struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

// Validate handles POST /app-templates/validate — parses the YAML and checks required fields.
func (h *AppTemplatesHandler) Validate(w http.ResponseWriter, r *http.Request) {
	var req validateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	errs := validateAppTemplateYAML(req.YAML)
	writeJSON(w, http.StatusOK, validateResponse{
		Valid:  len(errs) == 0,
		Errors: errs,
	})
}

// validateAppTemplateYAML parses the YAML string and checks required fields.
// Returns a list of validation error strings; empty means valid.
func validateAppTemplateYAML(content string) []string {
	var errs []string

	if content == "" {
		return []string{"YAML content is empty"}
	}

	var t apptemplate.AppTemplate
	if err := yaml.Unmarshal([]byte(content), &t); err != nil {
		return []string{"YAML parse error: " + err.Error()}
	}

	if t.Meta.Name == "" {
		errs = append(errs, "meta.name is required")
	}
	if t.Meta.Category == "" {
		errs = append(errs, "meta.category is required")
	}
	if t.Meta.Description == "" {
		errs = append(errs, "meta.description is required")
	}
	if t.Meta.Capability == "" {
		errs = append(errs, "meta.capability is required")
	}

	return errs
}

// createCustomRequest is the body for POST /app-templates/custom.
type createCustomRequest struct {
	YAML string `json:"yaml"`
}

// CreateCustom handles POST /app-templates/custom — validates and persists a custom app template.
func (h *AppTemplatesHandler) CreateCustom(w http.ResponseWriter, r *http.Request) {
	var req createCustomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	errs := validateAppTemplateYAML(req.YAML)
	if len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, validateResponse{
			Valid:  false,
			Errors: errs,
		})
		return
	}

	// Extract name from YAML for the record.
	var t apptemplate.AppTemplate
	_ = yaml.Unmarshal([]byte(req.YAML), &t)

	cp := &models.CustomProfile{
		ID:          uuid.New().String(),
		Name:        t.Meta.Name,
		YAMLContent: req.YAML,
	}

	if err := h.customProfiles.Create(r.Context(), cp); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save custom app template")
		return
	}

	writeJSON(w, http.StatusCreated, cp)
}

// ListCustom handles GET /app-templates/custom — returns all custom app templates.
func (h *AppTemplatesHandler) ListCustom(w http.ResponseWriter, r *http.Request) {
	items, err := h.customProfiles.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list custom app templates")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  items,
		"total": len(items),
	})
}
