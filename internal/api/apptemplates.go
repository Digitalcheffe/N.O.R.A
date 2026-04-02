package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// AppTemplatesHandler serves the app template library API.
type AppTemplatesHandler struct {
	registry  *apptemplate.Registry
	customDir string
}

// NewProfilesHandler creates an AppTemplatesHandler backed by the given registry and
// custom template directory. Custom templates are stored as YAML files in customDir.
func NewProfilesHandler(registry *apptemplate.Registry, customDir string) *AppTemplatesHandler {
	return &AppTemplatesHandler{registry: registry, customDir: customDir}
}

// Routes registers app template endpoints on r.
func (h *AppTemplatesHandler) Routes(r chi.Router) {
	r.Get("/app-templates", h.List)
	r.Post("/app-templates/reload", h.Reload)
	r.Post("/app-templates/validate", h.Validate)
	r.Post("/app-templates/custom", h.CreateCustom)
	r.Get("/app-templates/custom", h.ListCustom)
	r.Delete("/app-templates/custom/{id}", h.DeleteCustom)
	r.Get("/app-templates/{id}", h.Get)
}

// appTemplateMeta is the list-response shape — meta fields only, no internals.
type appTemplateMeta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Logo        string `json:"logo"`
	Icon        string `json:"icon,omitempty"` // CDN icon slug override; falls back to ID on the client
	Description string `json:"description"`
	Capability  string `json:"capability"`
	Homepage    string `json:"homepage,omitempty"`
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
			Icon:        t.Meta.Icon,
			Description: t.Meta.Description,
			Capability:  t.Meta.Capability,
			Homepage:    t.Meta.Homepage,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  items,
		"total": len(items),
	})
}

// Get handles GET /app-templates/{id} — returns meta fields for the template.
func (h *AppTemplatesHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	t, err := h.registry.Get(id)
	if err != nil || t == nil {
		writeError(w, http.StatusNotFound, "app template not found")
		return
	}

	writeJSON(w, http.StatusOK, appTemplateMeta{
		ID:          id,
		Name:        t.Meta.Name,
		Category:    t.Meta.Category,
		Logo:        t.Meta.Logo,
		Icon:        t.Meta.Icon,
		Description: t.Meta.Description,
		Capability:  t.Meta.Capability,
		Homepage:    t.Meta.Homepage,
	})
}

// Reload handles POST /app-templates/reload — re-reads all templates from disk.
func (h *AppTemplatesHandler) Reload(w http.ResponseWriter, r *http.Request) {
	if err := h.registry.Reload(); err != nil {
		writeError(w, http.StatusInternalServerError, "reload failed: "+err.Error())
		return
	}
	all := h.registry.List()
	writeJSON(w, http.StatusOK, map[string]interface{}{"loaded": len(all)})
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

// CreateCustom handles POST /app-templates/custom — validates and persists a custom app template as a YAML file.
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

	var t apptemplate.AppTemplate
	_ = yaml.Unmarshal([]byte(req.YAML), &t)

	id := uuid.New().String()
	path := filepath.Join(h.customDir, id+".yaml")
	if err := os.WriteFile(path, []byte(req.YAML), 0644); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save custom app template")
		return
	}

	// Reload so the new template is immediately available in the registry.
	_ = h.registry.Reload()

	cp := &models.CustomProfile{
		ID:          id,
		Name:        t.Meta.Name,
		YAMLContent: req.YAML,
		CreatedAt:   time.Now(),
	}
	writeJSON(w, http.StatusCreated, cp)
}

// ListCustom handles GET /app-templates/custom — returns all custom app templates from disk.
func (h *AppTemplatesHandler) ListCustom(w http.ResponseWriter, r *http.Request) {
	items, err := readCustomProfiles(h.customDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list custom app templates")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":  items,
		"total": len(items),
	})
}

// DeleteCustom handles DELETE /app-templates/custom/{id} — removes a custom app template file.
func (h *AppTemplatesHandler) DeleteCustom(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	path := filepath.Join(h.customDir, id+".yaml")

	if err := os.Remove(path); errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "custom app template not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete custom app template")
		return
	}

	// Reload so the deleted template is no longer available in the registry.
	_ = h.registry.Reload()

	w.WriteHeader(http.StatusNoContent)
}

// readCustomProfiles reads all *.yaml files from dir and returns them as CustomProfile records.
func readCustomProfiles(dir string) ([]models.CustomProfile, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []models.CustomProfile{}, nil
	}
	if err != nil {
		return nil, err
	}

	var out []models.CustomProfile
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var t apptemplate.AppTemplate
		if err := yaml.Unmarshal(data, &t); err != nil {
			continue
		}
		info, _ := e.Info()
		var modTime time.Time
		if info != nil {
			modTime = info.ModTime()
		}
		out = append(out, models.CustomProfile{
			ID:          e.Name()[:len(e.Name())-5], // strip ".yaml"
			Name:        t.Meta.Name,
			YAMLContent: string(data),
			CreatedAt:   modTime,
		})
	}
	return out, nil
}
