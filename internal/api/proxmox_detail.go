package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

// ProxmoxDetailHandler serves live Proxmox detail endpoints by proxying
// to the Proxmox API using the stored credentials for each component.
type ProxmoxDetailHandler struct {
	components repo.InfraComponentRepo
}

// NewProxmoxDetailHandler creates a handler wired to the infrastructure repo.
func NewProxmoxDetailHandler(components repo.InfraComponentRepo) *ProxmoxDetailHandler {
	return &ProxmoxDetailHandler{components: components}
}

// Routes registers all Proxmox detail endpoints.
func (h *ProxmoxDetailHandler) Routes(r chi.Router) {
	r.Get("/infrastructure/proxmox/{id}/storage", h.GetStorage)
	r.Get("/infrastructure/proxmox/{id}/guests",  h.GetGuests)
	r.Get("/infrastructure/proxmox/{id}/status",  h.GetNodeStatus)
	r.Get("/infrastructure/proxmox/{id}/tasks",   h.GetTaskFailures)
	r.Get("/infrastructure/proxmox/{id}/backups", h.GetBackupJobs)
	r.Get("/infrastructure/proxmox/{id}/backup-files", h.GetBackupFiles)
}

// getPoller loads the component, validates it is a proxmox_node with credentials,
// and returns a ProxmoxPoller ready to make API calls.
func (h *ProxmoxDetailHandler) getPoller(ctx context.Context, id string) (*infra.ProxmoxPoller, error) {
	c, err := h.components.Get(ctx, id)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, repo.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if c.Type != "proxmox_node" {
		return nil, fmt.Errorf("component is not a proxmox_node")
	}
	if c.Credentials == nil || *c.Credentials == "" {
		return nil, fmt.Errorf("no credentials configured for this component")
	}
	return infra.NewProxmoxPoller(id, *c.Credentials)
}

// GetStorage returns storage pool details from the Proxmox API.
// GET /api/v1/infrastructure/proxmox/{id}/storage
func (h *ProxmoxDetailHandler) GetStorage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	poller, err := h.getPoller(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "component not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	pools, err := poller.FetchStoragePools(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("proxmox api: %s", err.Error()))
		return
	}
	if pools == nil {
		pools = []infra.ProxmoxStoragePool{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": pools, "total": len(pools)})
}

// GetGuests returns VM and LXC inventory with extended detail from the Proxmox API.
// GET /api/v1/infrastructure/proxmox/{id}/guests
func (h *ProxmoxDetailHandler) GetGuests(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	poller, err := h.getPoller(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "component not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	guests, err := poller.FetchGuests(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("proxmox api: %s", err.Error()))
		return
	}
	if guests == nil {
		guests = []infra.ProxmoxGuestInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": guests, "total": len(guests)})
}

// GetNodeStatus returns extended node status including updates available.
// GET /api/v1/infrastructure/proxmox/{id}/status
func (h *ProxmoxDetailHandler) GetNodeStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	poller, err := h.getPoller(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "component not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	statuses, err := poller.FetchNodeStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("proxmox api: %s", err.Error()))
		return
	}
	if statuses == nil {
		statuses = []infra.ProxmoxNodeStatusDetail{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": statuses, "total": len(statuses)})
}

// GetBackupJobs returns recent vzdump backup tasks (all statuses) from the Proxmox API.
// GET /api/v1/infrastructure/proxmox/{id}/backups
func (h *ProxmoxDetailHandler) GetBackupJobs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	poller, err := h.getPoller(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "component not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	jobs, err := poller.FetchBackupJobs(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("proxmox api: %s", err.Error()))
		return
	}
	if jobs == nil {
		jobs = []infra.ProxmoxBackupJob{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": jobs, "total": len(jobs)})
}

// GetBackupFiles returns per-VM backup files from storage.
// GET /api/v1/infrastructure/proxmox/{id}/backup-files
func (h *ProxmoxDetailHandler) GetBackupFiles(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	poller, err := h.getPoller(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "component not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	files, err := poller.FetchBackupFiles(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("proxmox api: %s", err.Error()))
		return
	}
	if files == nil {
		files = []infra.ProxmoxBackupFile{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": files, "total": len(files)})
}

// GetTaskFailures returns recent failed tasks from the Proxmox API.
// GET /api/v1/infrastructure/proxmox/{id}/tasks
func (h *ProxmoxDetailHandler) GetTaskFailures(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	poller, err := h.getPoller(r.Context(), id)
	if errors.Is(err, repo.ErrNotFound) {
		writeError(w, http.StatusNotFound, "component not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	failures, err := poller.FetchTaskFailures(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("proxmox api: %s", err.Error()))
		return
	}
	if failures == nil {
		failures = []infra.ProxmoxTaskFailure{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": failures, "total": len(failures)})
}
