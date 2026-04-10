package daily

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/docker/docker/api/types/container"
	dockerimage "github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"

	"github.com/digitalcheffe/nora/internal/infra"
	"github.com/digitalcheffe/nora/internal/repo"
)

// imageUpdateAPI is the minimal Docker client subset used by ImageUpdatePoller,
// enabling mock injection in tests.
type imageUpdateAPI interface {
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
	ImageInspect(ctx context.Context, imageID string, opts ...dockerclient.ImageInspectOption) (dockerimage.InspectResponse, error)
}

// latestDigester is the registry lookup surface used by ImageUpdatePoller.
// *infra.RegistryClient satisfies this interface; tests inject a mock.
type latestDigester interface {
	GetLatestDigest(ctx context.Context, image string) (string, error)
}

// ImageUpdatePoller checks container registries once per day to detect whether
// a newer image is available for each running discovered container. It is
// purely informational — it never pulls or updates images.
//
// Both Docker Engine and Portainer containers are supported:
//   - Docker Engine containers: local manifest digest fetched via Docker socket.
//   - Portainer containers: local manifest digest stored by the Portainer
//     enrichment worker; used directly without a socket call.
type ImageUpdatePoller struct {
	store    *repo.Store
	registry latestDigester
	client   imageUpdateAPI
}

// NewImageUpdatePoller returns an ImageUpdatePoller connected to the local
// Docker daemon. It returns an error only if the Docker client cannot be
// constructed (socket absent, etc.).
func NewImageUpdatePoller(store *repo.Store) (*ImageUpdatePoller, error) {
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &ImageUpdatePoller{
		store:    store,
		registry: infra.NewRegistryClient(),
		client:   cli,
	}, nil
}

// newImageUpdatePollerWithClient creates an ImageUpdatePoller with an injected
// client and registry (for tests).
func newImageUpdatePollerWithClient(store *repo.Store, client imageUpdateAPI, registry latestDigester) *ImageUpdatePoller {
	return &ImageUpdatePoller{store: store, registry: registry, client: client}
}

// Run performs one full image update check cycle across all running discovered
// containers.
func (p *ImageUpdatePoller) Run(ctx context.Context) error {
	containers, err := p.store.DiscoveredContainers.ListAllDiscoveredContainers(ctx)
	if err != nil {
		return fmt.Errorf("list discovered containers: %w", err)
	}

	checked := 0
	updatesAvailable := 0

	for _, c := range containers {
		// Only poll running containers.
		if c.Status != "running" {
			continue
		}

		// Step 1: Get image digest — try Docker socket first, fall back to stored digest.
		imageDigest := ""
		imageName := c.Image // from DB (set during discovery)

		inspect, err := p.client.ContainerInspect(ctx, c.ContainerID)
		if err != nil {
			// Docker socket unavailable for this container (e.g. Portainer-sourced).
			// Use the manifest digest stored by the enrichment worker if available.
			if c.ImageDigest != nil && *c.ImageDigest != "" {
				imageDigest = *c.ImageDigest
			} else {
				// No local digest available — skip until enrichment populates it.
				continue
			}
		} else {
			// Prefer the OCI manifest descriptor digest (Docker Engine 25+),
			// fall back to the image config hash (image ID).
			imageDigest = inspect.Image
			if inspect.ImageManifestDescriptor != nil {
				imageDigest = inspect.ImageManifestDescriptor.Digest.String()
			} else {
				if imgInfo, err := p.client.ImageInspect(ctx, inspect.Image); err == nil { //nolint:staticcheck
					if inspect.Config != nil && inspect.Config.Image != "" {
						imageName = inspect.Config.Image
					}
					if d := extractRepoDigest(imgInfo.RepoDigests, imageName); d != "" {
						imageDigest = d
					}
				}
			}
			if inspect.Config != nil && inspect.Config.Image != "" {
				imageName = inspect.Config.Image
			}
		}

		// Step 2: Fetch the latest manifest digest from the registry.
		registryDigest, err := p.registry.GetLatestDigest(ctx, imageName)
		if err != nil {
			log.Printf("image update poller: get digest for %s (%s): %v", c.ContainerName, imageName, err)
			// Skip — do not mutate image_update_available on transient errors.
			continue
		}

		// Step 3: Compare and persist.
		// Only set update_available=1 when both digests are non-empty and differ.
		updateAvailable := imageDigest != "" && registryDigest != "" && imageDigest != registryDigest

		if err := p.store.DiscoveredContainers.UpdateContainerImageCheck(
			ctx, c.ID, imageDigest, registryDigest, updateAvailable,
		); err != nil {
			log.Printf("image update poller: persist check for %s: %v", c.ContainerName, err)
			continue
		}

		// Persist restart policy — only available when we have a socket inspect.
		if inspect.ContainerJSONBase != nil && inspect.HostConfig != nil &&
			inspect.HostConfig.RestartPolicy.Name != "" {
			if err := p.store.DiscoveredContainers.UpdateContainerRestartPolicy(
				ctx, c.ID, string(inspect.HostConfig.RestartPolicy.Name),
			); err != nil {
				log.Printf("image update poller: persist restart policy for %s: %v", c.ContainerName, err)
			}
		}

		checked++
		if updateAvailable {
			updatesAvailable++
		}
	}

	log.Printf("image update poller: check complete: %d containers checked, %d updates available", checked, updatesAvailable)
	return nil
}

// extractRepoDigest returns the manifest digest (sha256:...) from a
// RepoDigests slice for the given image name.
func extractRepoDigest(repoDigests []string, imageName string) string {
	repoRef := imageName
	if i := strings.LastIndex(repoRef, ":"); i >= 0 {
		if !strings.Contains(repoRef[i+1:], "/") {
			repoRef = repoRef[:i]
		}
	}

	for _, d := range repoDigests {
		at := strings.LastIndex(d, "@")
		if at < 0 {
			continue
		}
		imgPart := d[:at]
		digestPart := d[at+1:]

		imgPartNorm := strings.TrimPrefix(imgPart, "docker.io/")
		repoNorm := strings.TrimPrefix(repoRef, "docker.io/")

		if imgPartNorm == repoNorm || strings.HasSuffix(imgPartNorm, "/"+repoNorm) {
			return digestPart
		}
	}

	if len(repoDigests) > 0 {
		if at := strings.LastIndex(repoDigests[0], "@"); at >= 0 {
			return repoDigests[0][at+1:]
		}
	}
	return ""
}
