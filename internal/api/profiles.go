package api

import (
	"net/http"

	"github.com/digitalcheffe/nora/internal/profile"
	"github.com/go-chi/chi/v5"
)

// ProfilesHandler serves the profile library API.
type ProfilesHandler struct {
	registry *profile.Registry
}

// NewProfilesHandler creates a ProfilesHandler backed by the given registry.
func NewProfilesHandler(registry *profile.Registry) *ProfilesHandler {
	return &ProfilesHandler{registry: registry}
}

// Routes registers GET /profiles and GET /profiles/{id}.
func (h *ProfilesHandler) Routes(r chi.Router) {
	r.Get("/profiles", h.List)
	r.Get("/profiles/{id}", h.Get)
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
	ID      string          `json:"id"`
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
