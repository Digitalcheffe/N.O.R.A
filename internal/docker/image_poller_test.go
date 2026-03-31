package docker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	dockerimage "github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// ── mock imageUpdateAPI ────────────────────────────────────────────────────────

type mockImageUpdateClient struct {
	inspects      map[string]container.InspectResponse
	inspectErr    error
	imgInspects   map[string]dockerimage.InspectResponse
	imgInspectErr error
}

func (m *mockImageUpdateClient) ContainerInspect(_ context.Context, id string) (container.InspectResponse, error) {
	if m.inspectErr != nil {
		return container.InspectResponse{}, m.inspectErr
	}
	if info, ok := m.inspects[id]; ok {
		return info, nil
	}
	return container.InspectResponse{}, errors.New("container not found: " + id)
}

func (m *mockImageUpdateClient) ImageInspect(_ context.Context, id string, _ ...dockerclient.ImageInspectOption) (dockerimage.InspectResponse, error) {
	if m.imgInspectErr != nil {
		return dockerimage.InspectResponse{}, m.imgInspectErr
	}
	if info, ok := m.imgInspects[id]; ok {
		return info, nil
	}
	return dockerimage.InspectResponse{}, errors.New("image not found: " + id)
}

// ── mock latestDigester ────────────────────────────────────────────────────────

type mockLatestDigester struct {
	digests map[string]string
	err     error
}

func (m *mockLatestDigester) GetLatestDigest(_ context.Context, image string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if d, ok := m.digests[image]; ok {
		return d, nil
	}
	return "", errors.New("no digest for " + image)
}

// ── mock repos ─────────────────────────────────────────────────────────────────

type mockInfraComponentRepo struct {
	components []models.InfrastructureComponent
}

func (r *mockInfraComponentRepo) List(_ context.Context) ([]models.InfrastructureComponent, error) {
	return r.components, nil
}
func (r *mockInfraComponentRepo) ListByParent(_ context.Context, _ string) ([]models.InfrastructureComponent, error) {
	return nil, nil
}
func (r *mockInfraComponentRepo) Get(_ context.Context, _ string) (*models.InfrastructureComponent, error) {
	return nil, repo.ErrNotFound
}
func (r *mockInfraComponentRepo) Create(_ context.Context, _ *models.InfrastructureComponent) error {
	return nil
}
func (r *mockInfraComponentRepo) Update(_ context.Context, _ *models.InfrastructureComponent) error {
	return nil
}
func (r *mockInfraComponentRepo) Delete(_ context.Context, _ string) error { return nil }
func (r *mockInfraComponentRepo) UpdateStatus(_ context.Context, _, _, _ string) error {
	return nil
}
func (r *mockInfraComponentRepo) UpdateSNMPMeta(_ context.Context, _, _ string) error { return nil }
func (r *mockInfraComponentRepo) UpdateSynologyMeta(_ context.Context, _, _ string) error {
	return nil
}

type imageCheckCall struct {
	id              string
	imageDigest     string
	registryDigest  string
	updateAvailable bool
}

type mockDiscoveredContainerRepo struct {
	containers  []*models.DiscoveredContainer
	imageChecks []imageCheckCall
}

func (r *mockDiscoveredContainerRepo) UpsertDiscoveredContainer(_ context.Context, _ *models.DiscoveredContainer) error {
	return nil
}
func (r *mockDiscoveredContainerRepo) ListDiscoveredContainers(_ context.Context, _ string) ([]*models.DiscoveredContainer, error) {
	return r.containers, nil
}
func (r *mockDiscoveredContainerRepo) ListAllDiscoveredContainers(_ context.Context) ([]*models.DiscoveredContainer, error) {
	return r.containers, nil
}
func (r *mockDiscoveredContainerRepo) GetDiscoveredContainer(_ context.Context, _ string) (*models.DiscoveredContainer, error) {
	return nil, repo.ErrNotFound
}
func (r *mockDiscoveredContainerRepo) FindByName(_ context.Context, _, _ string) (*models.DiscoveredContainer, error) {
	return nil, repo.ErrNotFound
}
func (r *mockDiscoveredContainerRepo) SetDiscoveredContainerApp(_ context.Context, _, _ string) error {
	return nil
}
func (r *mockDiscoveredContainerRepo) ClearDiscoveredContainerApp(_ context.Context, _ string) error {
	return nil
}
func (r *mockDiscoveredContainerRepo) UpdateDiscoveredContainerStatus(_ context.Context, _ string, _ string, _ time.Time) error {
	return nil
}
func (r *mockDiscoveredContainerRepo) MarkStoppedIfNotRunning(_ context.Context, _ string, _ []string) error {
	return nil
}
func (r *mockDiscoveredContainerRepo) DeleteDiscoveredContainer(_ context.Context, _ string) error {
	return nil
}
func (r *mockDiscoveredContainerRepo) UpdateContainerImageCheck(_ context.Context, id, imageDigest, registryDigest string, updateAvailable bool) error {
	r.imageChecks = append(r.imageChecks, imageCheckCall{
		id:              id,
		imageDigest:     imageDigest,
		registryDigest:  registryDigest,
		updateAvailable: updateAvailable,
	})
	return nil
}

// ── helpers ────────────────────────────────────────────────────────────────────

func makePollerStore(infraComponents []models.InfrastructureComponent, containers []*models.DiscoveredContainer) (*repo.Store, *mockDiscoveredContainerRepo) {
	dc := &mockDiscoveredContainerRepo{containers: containers}
	store := repo.NewStore(
		nil, nil, nil, nil, nil, nil,
		&mockInfraComponentRepo{components: infraComponents},
		nil, nil, nil, nil, nil, nil, nil, nil,
		dc, nil, nil, nil, nil,
	)
	return store, dc
}

func dockerEngineComponent() models.InfrastructureComponent {
	return models.InfrastructureComponent{ID: "engine-1", Type: "docker_engine"}
}

func makeInspect(containerID, imageID, imageName string, repoDigests []string) (map[string]container.InspectResponse, map[string]dockerimage.InspectResponse) {
	inspects := map[string]container.InspectResponse{
		containerID: {
			ContainerJSONBase: &container.ContainerJSONBase{Image: imageID},
			Config:            &container.Config{Image: imageName},
		},
	}
	imgInspects := map[string]dockerimage.InspectResponse{
		imageID: {RepoDigests: repoDigests},
	}
	return inspects, imgInspects
}

// ── gate check: no Docker Engine components ────────────────────────────────────

func TestImageUpdatePoller_GateCheck_NoEngines(t *testing.T) {
	store, dc := makePollerStore(nil, []*models.DiscoveredContainer{
		{ID: "c1", ContainerID: "abc", ContainerName: "sonarr", Image: "linuxserver/sonarr:latest", Status: "running"},
	})

	reg := &mockLatestDigester{digests: map[string]string{"linuxserver/sonarr:latest": "sha256:new"}}
	p := newImageUpdatePollerWithClient(store, &mockImageUpdateClient{}, reg)

	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dc.imageChecks) != 0 {
		t.Errorf("expected 0 image checks when no Docker Engine configured, got %d", len(dc.imageChecks))
	}
}

// ── digest mismatch → update_available=true ────────────────────────────────────

func TestImageUpdatePoller_DigestMismatch(t *testing.T) {
	const (
		localManifest    = "sha256:aaaaaaaaaaaa"
		registryManifest = "sha256:bbbbbbbbbbbb"
		containerID      = "ctr-abc"
		imageName        = "linuxserver/sonarr:latest"
	)

	store, dc := makePollerStore(
		[]models.InfrastructureComponent{dockerEngineComponent()},
		[]*models.DiscoveredContainer{
			{ID: "c1", ContainerID: containerID, ContainerName: "sonarr", Image: imageName, Status: "running"},
		},
	)

	inspects, imgInspects := makeInspect(containerID, "sha256:cfg1", imageName,
		[]string{"linuxserver/sonarr@" + localManifest})

	client := &mockImageUpdateClient{inspects: inspects, imgInspects: imgInspects}
	reg := &mockLatestDigester{digests: map[string]string{imageName: registryManifest}}
	p := newImageUpdatePollerWithClient(store, client, reg)

	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(dc.imageChecks) != 1 {
		t.Fatalf("expected 1 image check, got %d", len(dc.imageChecks))
	}
	check := dc.imageChecks[0]
	if !check.updateAvailable {
		t.Error("expected update_available=true when digests differ")
	}
	if check.registryDigest != registryManifest {
		t.Errorf("registry_digest: got %q, want %q", check.registryDigest, registryManifest)
	}
}

// ── digest match → update_available=false ─────────────────────────────────────

func TestImageUpdatePoller_DigestMatch(t *testing.T) {
	const (
		sameDigest  = "sha256:cccccccccccc"
		containerID = "ctr-xyz"
		imageName   = "ghcr.io/meeb/tubesync:latest"
	)

	store, dc := makePollerStore(
		[]models.InfrastructureComponent{dockerEngineComponent()},
		[]*models.DiscoveredContainer{
			{ID: "c2", ContainerID: containerID, ContainerName: "tubesync", Image: imageName, Status: "running"},
		},
	)

	inspects, imgInspects := makeInspect(containerID, "sha256:cfg2", imageName,
		[]string{"ghcr.io/meeb/tubesync@" + sameDigest})

	client := &mockImageUpdateClient{inspects: inspects, imgInspects: imgInspects}
	reg := &mockLatestDigester{digests: map[string]string{imageName: sameDigest}}
	p := newImageUpdatePollerWithClient(store, client, reg)

	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(dc.imageChecks) != 1 {
		t.Fatalf("expected 1 image check, got %d", len(dc.imageChecks))
	}
	if dc.imageChecks[0].updateAvailable {
		t.Error("expected update_available=false when digests match")
	}
}

// ── registry error → UpdateContainerImageCheck not called ─────────────────────

func TestImageUpdatePoller_RegistryError_NoUpdateSet(t *testing.T) {
	const containerID = "ctr-err"

	store, dc := makePollerStore(
		[]models.InfrastructureComponent{dockerEngineComponent()},
		[]*models.DiscoveredContainer{
			{ID: "c3", ContainerID: containerID, ContainerName: "myapp", Image: "myapp:latest", Status: "running"},
		},
	)

	inspects, imgInspects := makeInspect(containerID, "sha256:cfg3", "myapp:latest",
		[]string{"myapp@sha256:local"})

	client := &mockImageUpdateClient{inspects: inspects, imgInspects: imgInspects}
	reg := &mockLatestDigester{err: errors.New("rate limited (429)")}
	p := newImageUpdatePollerWithClient(store, client, reg)

	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	// UpdateContainerImageCheck must NOT be called on registry error.
	if len(dc.imageChecks) != 0 {
		t.Errorf("expected 0 image check calls on registry error, got %d", len(dc.imageChecks))
	}
}

// ── stopped/exited containers skipped ────────────────────────────────────────

func TestImageUpdatePoller_NonRunningContainersSkipped(t *testing.T) {
	store, dc := makePollerStore(
		[]models.InfrastructureComponent{dockerEngineComponent()},
		[]*models.DiscoveredContainer{
			{ID: "c4", ContainerID: "s1", ContainerName: "stopped-app", Image: "myapp:latest", Status: "stopped"},
			{ID: "c5", ContainerID: "s2", ContainerName: "exited-app", Image: "myapp:latest", Status: "exited"},
		},
	)

	reg := &mockLatestDigester{digests: map[string]string{"myapp:latest": "sha256:digest"}}
	p := newImageUpdatePollerWithClient(store, &mockImageUpdateClient{}, reg)

	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(dc.imageChecks) != 0 {
		t.Errorf("expected 0 image checks for non-running containers, got %d", len(dc.imageChecks))
	}
}

// ── extractRepoDigest ─────────────────────────────────────────────────────────

func TestExtractRepoDigest(t *testing.T) {
	cases := []struct {
		name        string
		repoDigests []string
		imageName   string
		want        string
	}{
		{
			name:        "exact match",
			repoDigests: []string{"linuxserver/sonarr@sha256:abc123"},
			imageName:   "linuxserver/sonarr:latest",
			want:        "sha256:abc123",
		},
		{
			name:        "ghcr with tag",
			repoDigests: []string{"ghcr.io/meeb/tubesync@sha256:def456"},
			imageName:   "ghcr.io/meeb/tubesync:latest",
			want:        "sha256:def456",
		},
		{
			name:        "docker.io prefix stripped",
			repoDigests: []string{"docker.io/library/ubuntu@sha256:fff000"},
			imageName:   "library/ubuntu:22.04",
			want:        "sha256:fff000",
		},
		{
			name:        "fallback to first entry",
			repoDigests: []string{"other/image@sha256:fallback"},
			imageName:   "completely-different:latest",
			want:        "sha256:fallback",
		},
		{
			name:        "empty repo digests returns empty",
			repoDigests: []string{},
			imageName:   "anything:latest",
			want:        "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractRepoDigest(tc.repoDigests, tc.imageName)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
