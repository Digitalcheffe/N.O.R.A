package api

import (
	"fmt"
	"net/http"

	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/icons"
	"github.com/go-chi/chi/v5"
)

const cdnBase = "https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons/svg"

// IconsHandler serves cached app icons.
type IconsHandler struct {
	fetcher  *icons.Fetcher
	profiler apptemplate.Loader // used to resolve CDN slug when icon is not yet cached
}

// NewIconsHandler creates an IconsHandler.
func NewIconsHandler(f *icons.Fetcher, profiler apptemplate.Loader) *IconsHandler {
	return &IconsHandler{fetcher: f, profiler: profiler}
}

// Routes registers the icons endpoint.
func (h *IconsHandler) Routes(r chi.Router) {
	r.Get("/icons/{name}", h.Serve)
}

// Serve handles GET /api/v1/icons/{name}
// Serves the cached SVG. If not yet cached, redirects the browser directly to
// the dashboard-icons CDN using the template's icon slug so the icon shows
// immediately, and triggers a background fetch so subsequent requests are local.
func (h *IconsHandler) Serve(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	if h.fetcher.ServeIcon(w, name) {
		return
	}

	// Icon not cached yet — resolve the CDN slug from the template if available.
	slug := name
	if h.profiler != nil {
		if t, err := h.profiler.Get(name); err == nil && t != nil && t.Meta.Icon != "" {
			slug = t.Meta.Icon
		}
	}

	// Kick off a background fetch so the next request is served locally.
	h.fetcher.EnsureIcon(name, slug)

	// Redirect the browser to the CDN so the icon renders immediately.
	http.Redirect(w, r, fmt.Sprintf("%s/%s.svg", cdnBase, slug), http.StatusTemporaryRedirect)
}
