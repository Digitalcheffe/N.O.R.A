package api

import (
	"encoding/json"
	"net/http"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/profile"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// ProfilesHandler serves the profile library API.
type ProfilesHandler struct {
	registry       *profile.Registry
	customProfiles repo.CustomProfileRepo
}

// NewProfilesHandler creates a ProfilesHandler backed by the given registry and custom-profile repo.
func NewProfilesHandler(registry *profile.Registry, customProfiles repo.CustomProfileRepo) *ProfilesHandler {
	return &ProfilesHandler{registry: registry, customProfiles: customProfiles}
}

// Routes registers profile endpoints on r.
func (h *ProfilesHandler) Routes(r chi.Router) {
	r.Get("/profiles", h.List)
	r.Get("/profiles/{id}", h.Get)
	r.Post("/profiles/validate", h.Validate)
	r.Post("/profiles/custom", h.CreateCustom)
	r.Get("/profiles/custom", h.ListCustom)
}

// profileMeta is the list-response shape — meta fields only, no internals.
type profileMeta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Logo        string `json:"logo"`
	Description string `json:"description"`
	Capability  string `json:"capability"`
}

// profileDetail is the full response shape returned by GET /profiles/{id}.
type profileDetail struct {
	ID      string           `json:"id"`
	Profile *profile.Profile `json:"profile"`
}

// List handles GET /profiles — returns meta for all registered profiles.
func (h *ProfilesHandler) List(w http.ResponseWriter, r *http.Request) {
	all := h.registry.List()

	items := make([]profileMeta, 0, len(all))
	for id, p := range all {
		items = append(items, profileMeta{
			ID:          id,
			Name:        p.Meta.Name,
			Category:    p.Meta.Category,
			Logo:        p.Meta.Logo,
			Description: p.Meta.Description,
			Capability:  p.Meta.Capability,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  items,
		"total": len(items),
	})
}

// Get handles GET /profiles/{id} — returns the full profile including setup instructions.
func (h *ProfilesHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	p, err := h.registry.Get(id)
	if err != nil || p == nil {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}

	writeJSON(w, http.StatusOK, profileDetail{
		ID:      id,
		Profile: p,
	})
}

// validateRequest is the body for POST /profiles/validate.
type validateRequest struct {
	YAML string `json:"yaml"`
}

// validateResponse is returned by POST /profiles/validate.
type validateResponse struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

// Validate handles POST /profiles/validate — parses the YAML and checks required fields.
func (h *ProfilesHandler) Validate(w http.ResponseWriter, r *http.Request) {
	var req validateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	errs := validateProfileYAML(req.YAML)
	writeJSON(w, http.StatusOK, validateResponse{
		Valid:  len(errs) == 0,
		Errors: errs,
	})
}

// validateProfileYAML parses the YAML string and checks required fields.
// Returns a list of validation error strings; empty means valid.
func validateProfileYAML(content string) []string {
	var errs []string

	if content == "" {
		return []string{"YAML content is empty"}
	}

	var p profile.Profile
	if err := yaml.Unmarshal([]byte(content), &p); err != nil {
		return []string{"YAML parse error: " + err.Error()}
	}

	if p.Meta.Name == "" {
		errs = append(errs, "meta.name is required")
	}
	if p.Meta.Category == "" {
		errs = append(errs, "meta.category is required")
	}
	if p.Meta.Description == "" {
		errs = append(errs, "meta.description is required")
	}
	if p.Meta.Capability == "" {
		errs = append(errs, "meta.capability is required")
	}

	return errs
}

// createCustomRequest is the body for POST /profiles/custom.
type createCustomRequest struct {
	YAML string `json:"yaml"`
}

// CreateCustom handles POST /profiles/custom — validates and persists a custom profile.
func (h *ProfilesHandler) CreateCustom(w http.ResponseWriter, r *http.Request) {
	var req createCustomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	errs := validateProfileYAML(req.YAML)
	if len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, validateResponse{
			Valid:  false,
			Errors: errs,
		})
		return
	}

	// Extract name from YAML for the record.
	var p profile.Profile
	_ = yaml.Unmarshal([]byte(req.YAML), &p)

	cp := &models.CustomProfile{
		ID:          uuid.New().String(),
		Name:        p.Meta.Name,
		YAMLContent: req.YAML,
	}

	if err := h.customProfiles.Create(r.Context(), cp); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save custom profile")
		return
	}

	writeJSON(w, http.StatusCreated, cp)
}

// ListCustom handles GET /profiles/custom — returns all custom profiles.
func (h *ProfilesHandler) ListCustom(w http.ResponseWriter, r *http.Request) {
	items, err := h.customProfiles.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list custom profiles")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  items,
		"total": len(items),
	})
}
