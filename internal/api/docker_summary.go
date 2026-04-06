package api

import (
	"context"
	"net/http"

	dockerclient "github.com/docker/docker/client"
	imagetypes "github.com/docker/docker/api/types/image"
	networktypes "github.com/docker/docker/api/types/network"
	volumetypes "github.com/docker/docker/api/types/volume"
	"github.com/go-chi/chi/v5"

	"github.com/digitalcheffe/nora/internal/repo"
)

// dockerEngineSummaryResponse matches the shape of PortainerEndpointSummary
// so the frontend can use the same StatCard component.
type dockerEngineSummaryResponse struct {
	ContainersRunning int   `json:"containers_running"`
	ContainersStopped int   `json:"containers_stopped"`
	ImagesTotal       int   `json:"images_total"`
	ImagesDangling    int   `json:"images_dangling"`
	ImagesDiskBytes   int64 `json:"images_disk_bytes"`
	VolumesTotal      int   `json:"volumes_total"`
	VolumesUnused     int   `json:"volumes_unused"`
	VolumesDiskBytes  int64 `json:"volumes_disk_bytes"`
	NetworksTotal     int   `json:"networks_total"`
}

// DockerSummaryHandler serves system-level stats for a docker_engine component.
type DockerSummaryHandler struct {
	store *repo.Store
}

func NewDockerSummaryHandler(store *repo.Store) *DockerSummaryHandler {
	return &DockerSummaryHandler{store: store}
}

func (h *DockerSummaryHandler) Routes(r chi.Router) {
	r.Get("/infrastructure/{id}/docker-summary", h.GetSummary)
}

// GetSummary queries the local Docker daemon for image, volume, and network
// counts and returns a summary matching the PortainerEndpointSummary shape.
// GET /api/v1/infrastructure/{id}/docker-summary
func (h *DockerSummaryHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	componentID := chi.URLParam(r, "id")

	comp, err := h.store.InfraComponents.Get(r.Context(), componentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "infrastructure component not found")
		return
	}
	if comp.Type != "docker_engine" {
		writeError(w, http.StatusBadRequest, "component is not a docker_engine")
		return
	}

	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not connect to Docker daemon")
		return
	}
	defer cli.Close()

	resp, err := buildDockerSummary(r.Context(), cli)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "docker summary failed: "+err.Error())
		return
	}

	// Overlay running/stopped counts from discovered_containers so the numbers
	// match what NORA has actually seen (handles daemon restarts gracefully).
	containers, _ := h.store.DiscoveredContainers.ListDiscoveredContainers(r.Context(), componentID)
	for _, c := range containers {
		if c.Status == "running" {
			resp.ContainersRunning++
		} else {
			resp.ContainersStopped++
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func buildDockerSummary(ctx context.Context, cli *dockerclient.Client) (*dockerEngineSummaryResponse, error) {
	resp := &dockerEngineSummaryResponse{}

	// ── Images ──────────────────────────────────────────────────────────────────
	imgs, err := cli.ImageList(ctx, imagetypes.ListOptions{All: false})
	if err != nil {
		return nil, err
	}
	resp.ImagesTotal = len(imgs)
	for _, img := range imgs {
		if len(img.RepoTags) == 0 || (len(img.RepoTags) == 1 && img.RepoTags[0] == "<none>:<none>") {
			resp.ImagesDangling++
		}
		resp.ImagesDiskBytes += img.Size
	}

	// ── Volumes ──────────────────────────────────────────────────────────────────
	vols, err := cli.VolumeList(ctx, volumetypes.ListOptions{})
	if err != nil {
		return nil, err
	}
	resp.VolumesTotal = len(vols.Volumes)
	for _, v := range vols.Volumes {
		if v.UsageData != nil {
			if v.UsageData.RefCount == 0 {
				resp.VolumesUnused++
			}
			if v.UsageData.Size > 0 {
				resp.VolumesDiskBytes += v.UsageData.Size
			}
		}
	}

	// ── Networks ─────────────────────────────────────────────────────────────────
	nets, err := cli.NetworkList(ctx, networktypes.ListOptions{})
	if err != nil {
		return nil, err
	}
	// Exclude Docker's built-in networks (bridge, host, none) from the count
	// to match what Portainer reports.
	builtIn := map[string]bool{"bridge": true, "host": true, "none": true}
	for _, n := range nets {
		if !builtIn[n.Name] {
			resp.NetworksTotal++
		}
	}
	return resp, nil
}
