package snapshot

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
// *docker.RegistryClient satisfies this interface; tests inject a mock.
type latestDigester interface {
	GetLatestDigest(ctx context.Context, image string) (string, error)
}

// ImageUpdatePoller polls container registries to detect whether a newer
// image is available for each running discovered container.  It is purely
// informational — it never pulls or updates images.
//
// Startup gate: if no infrastructure_components with type="docker_engine" exist,
// each Run call returns early without contacting any registry.
type ImageUpdatePoller struct {
	store    *repo.Store
	registry latestDigester
	client   imageUpdateAPI
}

// NewImageUpdatePoller returns an ImageUpdatePoller connected to the local
// Docker daemon.  It returns an error only if the Docker client cannot be
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
// containers.  It returns early if no Docker Engine infrastructure components
// are configured.
func (p *ImageUpdatePoller) Run(ctx context.Context) error {
	// Gate check — skip silently if no Docker Engine components are configured.
	components, err := p.store.InfraComponents.List(ctx)
	if err != nil {
		return fmt.Errorf("list infra components: %w", err)
	}
	hasDockerEngine := false
	for _, c := range components {
		if c.Type == "docker_engine" {
			hasDockerEngine = true
			break
		}
	}
	if !hasDockerEngine {
		log.Printf("image update poller: no Docker Engine components configured, skipping")
		return nil
	}

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

		// Step 1: Get image info from Docker socket.
		inspect, err := p.client.ContainerInspect(ctx, c.ContainerID)
		if err != nil {
			log.Printf("image update poller: inspect container %s (%s): %v", c.ContainerName, c.ContainerID, err)
			continue
		}

		// image_digest: prefer the OCI manifest descriptor digest (available on
		// Docker Engine 25+), fall back to the image config hash (image ID).
		imageDigest := inspect.Image // sha256 config hash — always present
		if inspect.ImageManifestDescriptor != nil {
			imageDigest = inspect.ImageManifestDescriptor.Digest.String()
		} else {
			// Try ImageInspect to find the manifest digest from RepoDigests.
			if imgInfo, err := p.client.ImageInspect(ctx, inspect.Image); err == nil { //nolint:staticcheck
				imageName := c.Image
				if inspect.Config != nil && inspect.Config.Image != "" {
					imageName = inspect.Config.Image
				}
				if d := extractRepoDigest(imgInfo.RepoDigests, imageName); d != "" {
					imageDigest = d
				}
			}
		}

		// Step 2: Determine image name/tag to look up in the registry.
		imageName := c.Image // from DB (set during discovery)
		if inspect.Config != nil && inspect.Config.Image != "" {
			imageName = inspect.Config.Image
		}

		// Step 3: Fetch the latest manifest digest from the registry.
		registryDigest, err := p.registry.GetLatestDigest(ctx, imageName)
		if err != nil {
			log.Printf("image update poller: get digest for %s (%s): %v", c.ContainerName, imageName, err)
			// Skip — do not mutate image_update_available on transient errors.
			continue
		}

		// Step 4: Compare and persist.
		// Only set update_available=1 when both digests are non-empty and differ.
		// This avoids false positives when we could only get a config hash locally
		// (config hash ≠ manifest hash by definition).
		updateAvailable := imageDigest != "" && registryDigest != "" && imageDigest != registryDigest

		if err := p.store.DiscoveredContainers.UpdateContainerImageCheck(
			ctx, c.ID, imageDigest, registryDigest, updateAvailable,
		); err != nil {
			log.Printf("image update poller: persist check for %s: %v", c.ContainerName, err)
			continue
		}

		// Persist restart policy — available from the inspect we already have.
		if inspect.HostConfig != nil && inspect.HostConfig.RestartPolicy.Name != "" {
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
	// Strip tag from imageName to get the repo reference.
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

	// Fallback: return the first entry's digest regardless of image match.
	if len(repoDigests) > 0 {
		if at := strings.LastIndex(repoDigests[0], "@"); at >= 0 {
			return repoDigests[0][at+1:]
		}
	}
	return ""
}

