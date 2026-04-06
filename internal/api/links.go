package api

import (
	"encoding/json"
	"net/http"

	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

// LinksHandler exposes the component_links table via REST.
type LinksHandler struct {
	store *repo.Store
}

// NewLinksHandler returns a LinksHandler backed by store.
func NewLinksHandler(store *repo.Store) *LinksHandler {
	return &LinksHandler{store: store}
}

// Routes registers the link endpoints under the provided chi router.
func (h *LinksHandler) Routes(r chi.Router) {
	r.Get("/links", h.ListChildren)
	r.Post("/links", h.SetParent)
	r.Delete("/links", h.RemoveParent)
}

// ListChildren handles GET /api/v1/links
// With ?parent_type=x&parent_id=y returns direct children of that parent.
// Without query params returns all links.
func (h *LinksHandler) ListChildren(w http.ResponseWriter, r *http.Request) {
	parentType := r.URL.Query().Get("parent_type")
	parentID := r.URL.Query().Get("parent_id")

	if parentType == "" && parentID == "" {
		links, err := h.store.ComponentLinks.ListAll(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list links")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": links})
		return
	}

	if parentType == "" || parentID == "" {
		writeError(w, http.StatusBadRequest, "parent_type and parent_id are required")
		return
	}

	links, err := h.store.ComponentLinks.GetChildren(r.Context(), parentType, parentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list children")
		return
	}
	writeJSON(w, http.StatusOK, links)
}

// SetParent handles POST /api/v1/links
// Body: {"parent_type":"...","parent_id":"...","child_type":"...","child_id":"..."}
func (h *LinksHandler) SetParent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ParentType string `json:"parent_type"`
		ParentID   string `json:"parent_id"`
		ChildType  string `json:"child_type"`
		ChildID    string `json:"child_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ParentType == "" || req.ParentID == "" || req.ChildType == "" || req.ChildID == "" {
		writeError(w, http.StatusBadRequest, "parent_type, parent_id, child_type, and child_id are required")
		return
	}

	if err := h.store.ComponentLinks.SetParent(r.Context(), req.ParentType, req.ParentID, req.ChildType, req.ChildID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set parent")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveParent handles DELETE /api/v1/links?child_type=x&child_id=y
func (h *LinksHandler) RemoveParent(w http.ResponseWriter, r *http.Request) {
	childType := r.URL.Query().Get("child_type")
	childID := r.URL.Query().Get("child_id")
	if childType == "" || childID == "" {
		writeError(w, http.StatusBadRequest, "child_type and child_id are required")
		return
	}

	if err := h.store.ComponentLinks.RemoveParent(r.Context(), childType, childID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove parent")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
