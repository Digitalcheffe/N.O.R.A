package api

import (
	"net/http"

	"github.com/digitalcheffe/nora/internal/icons"
	"github.com/go-chi/chi/v5"
)

// IconsHandler serves cached app icons.
type IconsHandler struct {
	fetcher *icons.Fetcher
}

// NewIconsHandler creates an IconsHandler.
func NewIconsHandler(f *icons.Fetcher) *IconsHandler {
	return &IconsHandler{fetcher: f}
}

// Routes registers the icons endpoint.
func (h *IconsHandler) Routes(r chi.Router) {
	r.Get("/icons/{name}", h.Serve)
}

// Serve handles GET /api/v1/icons/{name}
// Returns the cached SVG or 404 (and triggers a background fetch for next time).
func (h *IconsHandler) Serve(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	if !h.fetcher.ServeIcon(w, name) {
		h.fetcher.EnsureIcon(name) // fetch in background for next request
		writeError(w, http.StatusNotFound, "icon not available")
	}
}
