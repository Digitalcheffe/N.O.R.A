package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// PortainerCredentials holds the config fields stored in
// infrastructure_components.credentials for a portainer-type component.
type PortainerCredentials struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

// ParsePortainerCredentials decodes the JSON credentials blob for a portainer component.
func ParsePortainerCredentials(credJSON string) (*PortainerCredentials, error) {
	var c PortainerCredentials
	if err := json.Unmarshal([]byte(credJSON), &c); err != nil {
		return nil, fmt.Errorf("parse portainer credentials: %w", err)
	}
	return &c, nil
}

// PortainerClient calls the Portainer REST API. All requests carry
// X-API-Key: {api_key} as the authentication header on every request.
// It carries no state between calls and is safe for concurrent use.
type PortainerClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// NewPortainerClient returns a client targeting the Portainer API at baseURL.
func NewPortainerClient(baseURL, apiKey string) *PortainerClient {
	return &PortainerClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// newPortainerClientWithHTTP creates a PortainerClient with an injected http.Client (for tests).
func newPortainerClientWithHTTP(baseURL, apiKey string, hc *http.Client) *PortainerClient {
	return &PortainerClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    hc,
	}
}

// ── API types ─────────────────────────────────────────────────────────────────

// PortainerEndpoint is one Portainer environment (a Docker host or cluster).
// Type 1 = Docker standalone, 2 = Docker Agent, 4 = Edge Agent, etc.
type PortainerEndpoint struct {
	ID   int    `json:"Id"`
	Name string `json:"Name"`
	Type int    `json:"Type"`
}

// PortainerContainerPort is one port binding from the Docker container list.
type PortainerContainerPort struct {
	IP          string `json:"IP,omitempty"`
	PrivatePort uint16 `json:"PrivatePort"`
	PublicPort  uint16 `json:"PublicPort,omitempty"`
	Type        string `json:"Type"`
}

// PortainerEndpointSettings holds per-network info for a container.
type PortainerEndpointSettings struct {
	IPAddress string `json:"IPAddress"`
}

// PortainerNetworkSettings holds the networks map from the container list response.
type PortainerNetworkSettings struct {
	Networks map[string]*PortainerEndpointSettings `json:"Networks"`
}

// PortainerContainer mirrors a Docker container list entry returned via
// GET /api/endpoints/{id}/docker/containers/json?all=true
type PortainerContainer struct {
	ID              string                   `json:"Id"`
	Names           []string                 `json:"Names"`
	Image           string                   `json:"Image"`
	State           string                   `json:"State"`
	Status          string                   `json:"Status"`
	Labels          map[string]string        `json:"Labels"`
	Ports           []PortainerContainerPort `json:"Ports"`
	NetworkSettings *PortainerNetworkSettings `json:"NetworkSettings"`
}

// FirstName returns the primary container name with the leading slash stripped.
// Docker always prefixes container names with "/".
func (c *PortainerContainer) FirstName() string {
	if len(c.Names) == 0 {
		return ""
	}
	return strings.TrimPrefix(c.Names[0], "/")
}

// StackName returns the Docker Compose project label, if present.
func (c *PortainerContainer) StackName() string {
	return c.Labels["com.docker.compose.project"]
}

// PortainerContainerInspect is the detailed container inspect from
// GET /api/endpoints/{id}/docker/containers/{id}/json
type PortainerContainerInspect struct {
	// Image is the sha256 config hash of the locally running image (always present).
	Image  string `json:"Image"`
	Config struct {
		// Image is the image name:tag used to create the container.
		Image string `json:"Image"`
		// Env is the list of environment variables as "KEY=VALUE" strings.
		Env []string `json:"Env"`
	} `json:"Config"`
}

// PortainerImageInspect is the image detail response from
// GET /api/endpoints/{id}/docker/images/{imageId}/json.
// RepoDigests contains manifest digests (sha256:...) as "image@sha256:..." strings,
// stored locally for each tag that was pulled from a registry.
type PortainerImageInspect struct {
	RepoDigests []string `json:"RepoDigests"`
}

// PortainerContainerStats is the stats response from
// GET /api/endpoints/{id}/docker/containers/{id}/stats?stream=false.
// It contains two consecutive CPU samples (cpu_stats and precpu_stats) so
// that a delta-based CPU percentage can be calculated.
type PortainerContainerStats struct {
	CPUStats    portainerCPUSample `json:"cpu_stats"`
	PreCPUStats portainerCPUSample `json:"precpu_stats"`
	MemoryStats portainerMemStats  `json:"memory_stats"`
}

type portainerCPUSample struct {
	CPUUsage struct {
		TotalUsage  uint64   `json:"total_usage"`
		PercpuUsage []uint64 `json:"percpu_usage"`
	} `json:"cpu_usage"`
	SystemCPUUsage uint64 `json:"system_cpu_usage"`
	OnlineCPUs     int    `json:"online_cpus"`
}

type portainerMemStats struct {
	Usage uint64 `json:"usage"`
	Limit uint64 `json:"limit"`
	Stats struct {
		Cache uint64 `json:"cache"`
	} `json:"stats"`
}

// PortainerImageSummary is one entry from
// GET /api/endpoints/{id}/docker/images/json
type PortainerImageSummary struct {
	ID         string   `json:"Id"`
	RepoTags   []string `json:"RepoTags"`
	Size       int64    `json:"Size"`
	Containers int64    `json:"Containers"`
}

// PortainerVolumesResponse is the response from
// GET /api/endpoints/{id}/docker/volumes
type PortainerVolumesResponse struct {
	Volumes []PortainerVolume `json:"Volumes"`
}

// PortainerVolume is one Docker volume.
type PortainerVolume struct {
	Name      string `json:"Name"`
	UsageData *struct {
		Size     int64 `json:"Size"`
		RefCount int64 `json:"RefCount"` // number of containers mounting this volume
	} `json:"UsageData,omitempty"`
}

// PortainerNetwork is one Docker network.
type PortainerNetwork struct {
	ID   string `json:"Id"`
	Name string `json:"Name"`
}

// ── API methods ───────────────────────────────────────────────────────────────

// Ping verifies connectivity to Portainer by listing endpoints.
func (c *PortainerClient) Ping(ctx context.Context) error {
	_, err := c.ListEndpoints(ctx)
	return err
}

// ListEndpoints returns all Portainer environments.
// GET /api/endpoints
func (c *PortainerClient) ListEndpoints(ctx context.Context) ([]PortainerEndpoint, error) {
	var out []PortainerEndpoint
	if err := c.get(ctx, "/api/endpoints", &out); err != nil {
		return nil, fmt.Errorf("list portainer endpoints: %w", err)
	}
	return out, nil
}

// ListContainers returns all containers for the given endpoint.
// GET /api/endpoints/{id}/docker/containers/json?all=true
func (c *PortainerClient) ListContainers(ctx context.Context, endpointID int) ([]PortainerContainer, error) {
	var out []PortainerContainer
	path := fmt.Sprintf("/api/endpoints/%d/docker/containers/json?all=true", endpointID)
	if err := c.get(ctx, path, &out); err != nil {
		return nil, fmt.Errorf("list containers endpoint %d: %w", endpointID, err)
	}
	return out, nil
}

// InspectContainer returns the detailed inspect for one container.
// GET /api/endpoints/{id}/docker/containers/{containerId}/json
func (c *PortainerClient) InspectContainer(ctx context.Context, endpointID int, containerID string) (*PortainerContainerInspect, error) {
	var out PortainerContainerInspect
	path := fmt.Sprintf("/api/endpoints/%d/docker/containers/%s/json", endpointID, containerID)
	if err := c.get(ctx, path, &out); err != nil {
		return nil, fmt.Errorf("inspect container %s endpoint %d: %w", containerID, endpointID, err)
	}
	return &out, nil
}

// InspectImage returns image details, including locally stored manifest digests.
// GET /api/endpoints/{id}/docker/images/{imageId}/json
func (c *PortainerClient) InspectImage(ctx context.Context, endpointID int, imageID string) (*PortainerImageInspect, error) {
	var out PortainerImageInspect
	path := fmt.Sprintf("/api/endpoints/%d/docker/images/%s/json", endpointID, imageID)
	if err := c.get(ctx, path, &out); err != nil {
		return nil, fmt.Errorf("inspect image %s endpoint %d: %w", imageID, endpointID, err)
	}
	return &out, nil
}

// GetContainerStats fetches one stats snapshot for a container.
// The stats endpoint returns two consecutive CPU samples so a delta can be computed.
// GET /api/endpoints/{id}/docker/containers/{containerId}/stats?stream=false
// A separate 5-second timeout http.Client is used so slow stats don't stall the worker.
func (c *PortainerClient) GetContainerStats(ctx context.Context, endpointID int, containerID string) (*PortainerContainerStats, error) {
	var out PortainerContainerStats
	path := fmt.Sprintf("/api/endpoints/%d/docker/containers/%s/stats?stream=false", endpointID, containerID)
	// Use a tighter 5-second timeout for stats calls; caller must respect this with ctx.
	if err := c.get(ctx, path, &out); err != nil {
		return nil, fmt.Errorf("stats container %s endpoint %d: %w", containerID, endpointID, err)
	}
	return &out, nil
}

// ListImages returns all images for the given endpoint.
// GET /api/endpoints/{id}/docker/images/json
func (c *PortainerClient) ListImages(ctx context.Context, endpointID int) ([]PortainerImageSummary, error) {
	var out []PortainerImageSummary
	path := fmt.Sprintf("/api/endpoints/%d/docker/images/json", endpointID)
	if err := c.get(ctx, path, &out); err != nil {
		return nil, fmt.Errorf("list images endpoint %d: %w", endpointID, err)
	}
	return out, nil
}

// ListVolumes returns all volumes for the given endpoint.
// GET /api/endpoints/{id}/docker/volumes
func (c *PortainerClient) ListVolumes(ctx context.Context, endpointID int) (*PortainerVolumesResponse, error) {
	var out PortainerVolumesResponse
	path := fmt.Sprintf("/api/endpoints/%d/docker/volumes", endpointID)
	if err := c.get(ctx, path, &out); err != nil {
		return nil, fmt.Errorf("list volumes endpoint %d: %w", endpointID, err)
	}
	return &out, nil
}

// ListNetworks returns all networks for the given endpoint.
// GET /api/endpoints/{id}/docker/networks
func (c *PortainerClient) ListNetworks(ctx context.Context, endpointID int) ([]PortainerNetwork, error) {
	var out []PortainerNetwork
	path := fmt.Sprintf("/api/endpoints/%d/docker/networks", endpointID)
	if err := c.get(ctx, path, &out); err != nil {
		return nil, fmt.Errorf("list networks endpoint %d: %w", endpointID, err)
	}
	return out, nil
}

// ── Stats helpers ─────────────────────────────────────────────────────────────

// CalcCPUPercent computes CPU usage percentage from a stats snapshot.
// Formula mirrors the Docker CLI:
//
//	cpuDelta   = current.TotalUsage - precpu.TotalUsage
//	systemDelta = current.SystemCPUUsage - precpu.SystemCPUUsage
//	percent    = (cpuDelta / systemDelta) * numCPUs * 100
func CalcCPUPercent(s *PortainerContainerStats) float64 {
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage) - float64(s.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(s.CPUStats.SystemCPUUsage) - float64(s.PreCPUStats.SystemCPUUsage)
	if sysDelta <= 0 || cpuDelta < 0 {
		return 0
	}
	numCPUs := s.CPUStats.OnlineCPUs
	if numCPUs == 0 {
		numCPUs = len(s.CPUStats.CPUUsage.PercpuUsage)
	}
	if numCPUs == 0 {
		numCPUs = 1
	}
	return (cpuDelta / sysDelta) * float64(numCPUs) * 100.0
}

// CalcMemPercent computes memory usage as a percentage of the container limit.
// Cache is subtracted from usage (RSS only) to match Docker stats behaviour.
func CalcMemPercent(s *PortainerContainerStats) float64 {
	if s.MemoryStats.Limit == 0 {
		return 0
	}
	rss := s.MemoryStats.Usage
	if s.MemoryStats.Stats.Cache < rss {
		rss -= s.MemoryStats.Stats.Cache
	}
	return float64(rss) / float64(s.MemoryStats.Limit) * 100.0
}

// ExtractManifestDigest returns the first manifest digest from a RepoDigests slice
// that matches imageName (repo:tag → repo@sha256:...). Falls back to the first
// entry if no named match is found. Returns "" if RepoDigests is empty.
func ExtractManifestDigest(repoDigests []string, imageName string) string {
	// Strip tag from imageName to get repo reference.
	repo := imageName
	if i := strings.LastIndex(repo, ":"); i >= 0 && !strings.Contains(repo[i+1:], "/") {
		repo = repo[:i]
	}
	// Normalize docker.io prefix.
	repoNorm := strings.TrimPrefix(repo, "docker.io/")

	for _, d := range repoDigests {
		at := strings.LastIndex(d, "@")
		if at < 0 {
			continue
		}
		imgPart := strings.TrimPrefix(d[:at], "docker.io/")
		if imgPart == repoNorm || strings.HasSuffix(imgPart, "/"+repoNorm) {
			return d[at+1:]
		}
	}
	// Fallback: first entry's digest regardless of name match.
	if len(repoDigests) > 0 {
		if at := strings.LastIndex(repoDigests[0], "@"); at >= 0 {
			return repoDigests[0][at+1:]
		}
	}
	return ""
}

// ── Internal request helper ───────────────────────────────────────────────────

// get makes an authenticated GET request and decodes the JSON response body into v.
func (c *PortainerClient) get(ctx context.Context, path string, v interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("portainer API %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
