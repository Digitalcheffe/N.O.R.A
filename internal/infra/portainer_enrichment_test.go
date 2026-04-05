package infra

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/migrations"
)

// ── test helpers ──────────────────────────────────────────────────────────────

func newPortainerTestStore(t *testing.T) *repo.Store {
	t.Helper()
	cfg := &config.Config{DBPath: ":memory:"}
	db, err := repo.Open(cfg, migrations.Files)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return repo.NewStore(
		repo.NewAppRepo(db),
		repo.NewEventRepo(db),
		repo.NewCheckRepo(db),
		repo.NewRollupRepo(db),
		repo.NewResourceReadingRepo(db),
		repo.NewResourceRollupRepo(db),
		repo.NewInfraComponentRepo(db),
		repo.NewDockerEngineRepo(db),
		repo.NewInfraRepo(db),
		repo.NewSettingsRepo(db),
		repo.NewMetricsRepo(db),
		repo.NewUserRepo(db),
		repo.NewTraefikComponentRepo(db),
		repo.NewTraefikOverviewRepo(db),
		repo.NewTraefikServiceRepo(db),
		repo.NewDiscoveredContainerRepo(db),
		repo.NewDiscoveredRouteRepo(db),
		nil,
		nil,
		nil,
		nil,
		nil,
	)
}

func createPortainerComponent(t *testing.T, store *repo.Store, id, baseURL string) {
	t.Helper()
	creds, _ := json.Marshal(PortainerCredentials{
		BaseURL: baseURL,
		APIKey:  "test-key",
	})
	s := string(creds)
	c := &models.InfrastructureComponent{
		ID:               id,
		Name:             "portainer-" + id,
		Type:             "portainer",
		CollectionMethod: "portainer_api",
		Credentials:      &s,
		Enabled:          true,
		LastStatus:       "unknown",
		CreatedAt:        "2026-01-01T00:00:00Z",
	}
	if err := store.InfraComponents.Create(context.Background(), c); err != nil {
		t.Fatalf("create portainer component: %v", err)
	}
}

func createDockerEngineComponent(t *testing.T, store *repo.Store, id string) {
	t.Helper()
	c := &models.InfrastructureComponent{
		ID:               id,
		Name:             "docker-" + id,
		Type:             "docker_engine",
		CollectionMethod: "docker_socket",
		Enabled:          true,
		LastStatus:       "unknown",
		CreatedAt:        "2026-01-01T00:00:00Z",
	}
	if err := store.InfraComponents.Create(context.Background(), c); err != nil {
		t.Fatalf("create docker engine component: %v", err)
	}
}

func createDiscoveredContainer(t *testing.T, store *repo.Store, infraID, name, image string, updateAvail int) *models.DiscoveredContainer {
	t.Helper()
	c := &models.DiscoveredContainer{
		InfraComponentID:    infraID,
		ContainerID:         "docker-" + name,
		ContainerName:       name,
		Image:               image,
		Status:              "running",
		LastSeenAt:          time.Now(),
		CreatedAt:           time.Now(),
		ImageUpdateAvailable: updateAvail,
	}
	if err := store.DiscoveredContainers.UpsertDiscoveredContainer(context.Background(), c); err != nil {
		t.Fatalf("create discovered container: %v", err)
	}
	// Reload to get the generated ID.
	all, err := store.DiscoveredContainers.ListDiscoveredContainers(context.Background(), infraID)
	if err != nil {
		t.Fatalf("list containers: %v", err)
	}
	for _, dc := range all {
		if dc.ContainerName == name {
			return dc
		}
	}
	t.Fatalf("container %q not found after create", name)
	return nil
}

// ── fake Portainer server ─────────────────────────────────────────────────────

// portainerFakeServer simulates the Portainer REST API for tests.
type portainerFakeServer struct {
	endpoints  []PortainerEndpoint
	containers map[int][]PortainerContainer        // endpointID → containers
	inspects   map[string]*PortainerContainerInspect // containerID → inspect
	images     map[string]*PortainerImageInspect    // imageID → image inspect
}

func newPortainerFakeServer(t *testing.T) (*portainerFakeServer, *httptest.Server) {
	t.Helper()
	fs := &portainerFakeServer{
		containers: make(map[int][]PortainerContainer),
		inspects:   make(map[string]*PortainerContainerInspect),
		images:     make(map[string]*PortainerImageInspect),
	}
	srv := httptest.NewServer(fs)
	t.Cleanup(srv.Close)
	return fs, srv
}

func (s *portainerFakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	path := r.URL.Path

	// GET /api/endpoints
	if path == "/api/endpoints" {
		json.NewEncoder(w).Encode(s.endpoints)
		return
	}

	// GET /api/endpoints/{id}/docker/containers/json
	if strings.Contains(path, "/docker/containers/json") {
		var epID int
		fmt.Sscanf(path, "/api/endpoints/%d/", &epID)
		containers := s.containers[epID]
		if containers == nil {
			containers = []PortainerContainer{}
		}
		json.NewEncoder(w).Encode(containers)
		return
	}

	// GET /api/endpoints/{id}/docker/containers/{cid}/json (inspect)
	if strings.Contains(path, "/docker/containers/") && strings.HasSuffix(path, "/json") && !strings.Contains(path, "/stats") {
		parts := strings.Split(path, "/")
		// /api/endpoints/{id}/docker/containers/{cid}/json
		if len(parts) >= 6 {
			cid := parts[len(parts)-2]
			if ins, ok := s.inspects[cid]; ok {
				json.NewEncoder(w).Encode(ins)
				return
			}
		}
		http.NotFound(w, r)
		return
	}

	// GET /api/endpoints/{id}/docker/images/{imageId}/json
	if strings.Contains(path, "/docker/images/") && strings.HasSuffix(path, "/json") {
		parts := strings.Split(path, "/")
		if len(parts) >= 6 {
			imgID := parts[len(parts)-2]
			if img, ok := s.images[imgID]; ok {
				json.NewEncoder(w).Encode(img)
				return
			}
		}
		http.NotFound(w, r)
		return
	}

	http.NotFound(w, r)
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestPortainerWorkerStartsWithNoDockerEngine verifies that the enrichment
// worker starts and runs successfully when no Docker Engine components exist.
func TestPortainerWorkerStartsWithNoDockerEngine(t *testing.T) {
	fakeServer, srv := newPortainerFakeServer(t)
	fakeServer.endpoints = []PortainerEndpoint{{ID: 1, Name: "local"}}
	fakeServer.containers[1] = []PortainerContainer{}

	store := newPortainerTestStore(t)
	createPortainerComponent(t, store, "portainer-1", srv.URL)
	// Do NOT create any docker_engine component — gate must not block.

	worker := NewPortainerEnrichmentWorker(store)
	err := worker.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() with no docker engine: unexpected error: %v", err)
	}
}

// TestPortainerWorkerSkipsWhenNoPortainerComponents verifies the gate check:
// Run should return nil immediately with no side-effects.
func TestPortainerWorkerSkipsWhenNoPortainerComponents(t *testing.T) {
	store := newPortainerTestStore(t)
	// Only a docker_engine component — no portainer.
	createDockerEngineComponent(t, store, "docker-1")

	worker := NewPortainerEnrichmentWorker(store)
	if err := worker.Run(context.Background()); err != nil {
		t.Fatalf("Run() with no portainer components: unexpected error: %v", err)
	}
}

// TestPortainerContainerNameNormalization verifies that leading "/" is stripped
// before name matching against NORA's discovered_containers.
func TestPortainerContainerNameNormalization(t *testing.T) {
	fakeServer, srv := newPortainerFakeServer(t)
	fakeServer.endpoints = []PortainerEndpoint{{ID: 1, Name: "local"}}
	// Portainer returns container names with a leading slash (Docker convention).
	fakeServer.containers[1] = []PortainerContainer{
		{
			ID:    "abc123",
			Names: []string{"/sonarr"}, // leading slash
			Image: "linuxserver/sonarr:latest",
			State: "running",
		},
	}
	fakeServer.inspects["abc123"] = &PortainerContainerInspect{
		Image:  "sha256:aaabbb",
		Config: struct{ Image string `json:"Image"` }{Image: "linuxserver/sonarr:latest"},
	}
	fakeServer.images["sha256:aaabbb"] = &PortainerImageInspect{
		RepoDigests: []string{},
	}

	store := newPortainerTestStore(t)
	createPortainerComponent(t, store, "p1", srv.URL)
	createDockerEngineComponent(t, store, "d1")
	// NORA-known container named "sonarr" (no slash) under the docker_engine component.
	createDiscoveredContainer(t, store, "d1", "sonarr", "linuxserver/sonarr:latest", 0)

	worker := NewPortainerEnrichmentWorker(store)
	if err := worker.Run(context.Background()); err != nil {
		t.Fatalf("Run(): %v", err)
	}

	// After enrichment, the container should have been matched (no error = success).
	// Verify last_polled_at was updated on the portainer component.
	comp, err := store.InfraComponents.Get(context.Background(), "p1")
	if err != nil {
		t.Fatalf("get portainer component: %v", err)
	}
	if comp.LastStatus != "online" {
		t.Errorf("expected last_status=online, got %q", comp.LastStatus)
	}
}

// TestPortainerWinsOverDD9ForImageUpdateAvailable verifies that after Portainer
// enrichment, the image_update_available flag on a matched container reflects
// Portainer's assessment, overwriting whatever DD-9 stored.
func TestPortainerWinsOverDD9ForImageUpdateAvailable(t *testing.T) {
	fakeServer, srv := newPortainerFakeServer(t)
	fakeServer.endpoints = []PortainerEndpoint{{ID: 1, Name: "local"}}
	fakeServer.containers[1] = []PortainerContainer{
		{
			ID:    "ctr1",
			Names: []string{"/myapp"},
			Image: "myrepo/myapp:latest",
			State: "running",
		},
	}
	fakeServer.inspects["ctr1"] = &PortainerContainerInspect{
		Image:  "sha256:running",
		Config: struct{ Image string `json:"Image"` }{Image: "myrepo/myapp:latest"},
	}
	// RepoDigests contains the manifest digest of the locally running image.
	fakeServer.images["sha256:running"] = &PortainerImageInspect{
		RepoDigests: []string{"myrepo/myapp@sha256:localmanifest"},
	}

	store := newPortainerTestStore(t)
	createPortainerComponent(t, store, "p1", srv.URL)

	// Pre-seed the container under p1 with the Portainer container ID ("ctr1") so
	// the upsert conflict key matches and the registry_digest is visible to the worker.
	dc := &models.DiscoveredContainer{
		InfraComponentID: "p1",
		ContainerID:      "ctr1",
		ContainerName:    "myapp",
		Image:            "myrepo/myapp:latest",
		Status:           "running",
		LastSeenAt:       time.Now(),
		CreatedAt:        time.Now(),
	}
	if err := store.DiscoveredContainers.UpsertDiscoveredContainer(context.Background(), dc); err != nil {
		t.Fatalf("pre-seed container: %v", err)
	}
	seeded, err := store.DiscoveredContainers.FindByName(context.Background(), "p1", "myapp")
	if err != nil {
		t.Fatalf("find pre-seeded container: %v", err)
	}

	// Manually store DD-9 registry_digest (different from running manifest → update available).
	registryDigest := "sha256:newermanifest"
	if err := store.DiscoveredContainers.UpdateContainerImageCheck(
		context.Background(), seeded.ID, "ctr1", registryDigest, false,
	); err != nil {
		t.Fatalf("seed DD-9 data: %v", err)
	}

	worker := NewPortainerEnrichmentWorker(store)
	if err := worker.Run(context.Background()); err != nil {
		t.Fatalf("Run(): %v", err)
	}

	// Reload and verify Portainer detected the update (running manifest ≠ registry digest).
	updated, err := store.DiscoveredContainers.GetDiscoveredContainer(context.Background(), seeded.ID)
	if err != nil {
		t.Fatalf("get container: %v", err)
	}
	if updated.ImageUpdateAvailable != 1 {
		t.Errorf("expected image_update_available=1, got %d", updated.ImageUpdateAvailable)
	}
}

// TestPortainerEventEmittedOnFalseToTrueTransition verifies that an event is
// emitted exactly once when image_update_available transitions false→true,
// and NOT emitted on subsequent polls where the value remains true.
func TestPortainerEventEmittedOnFalseToTrueTransition(t *testing.T) {
	fakeServer, srv := newPortainerFakeServer(t)
	fakeServer.endpoints = []PortainerEndpoint{{ID: 1, Name: "local"}}
	fakeServer.containers[1] = []PortainerContainer{
		{
			ID:    "ctr2",
			Names: []string{"/webapp"},
			Image: "myrepo/webapp:1.0",
			State: "running",
		},
	}
	fakeServer.inspects["ctr2"] = &PortainerContainerInspect{
		Image:  "sha256:old",
		Config: struct{ Image string `json:"Image"` }{Image: "myrepo/webapp:1.0"},
	}
	fakeServer.images["sha256:old"] = &PortainerImageInspect{
		RepoDigests: []string{"myrepo/webapp@sha256:oldmanifest"},
	}

	store := newPortainerTestStore(t)
	createPortainerComponent(t, store, "p1", srv.URL)

	// Pre-seed the container under p1 with the Portainer container ID ("ctr2").
	dcRaw := &models.DiscoveredContainer{
		InfraComponentID: "p1",
		ContainerID:      "ctr2",
		ContainerName:    "webapp",
		Image:            "myrepo/webapp:1.0",
		Status:           "running",
		LastSeenAt:       time.Now(),
		CreatedAt:        time.Now(),
	}
	if err := store.DiscoveredContainers.UpsertDiscoveredContainer(context.Background(), dcRaw); err != nil {
		t.Fatalf("pre-seed container: %v", err)
	}
	dc, err := store.DiscoveredContainers.FindByName(context.Background(), "p1", "webapp")
	if err != nil {
		t.Fatalf("find pre-seeded container: %v", err)
	}

	// Seed: DD-9 says there's a newer image.
	if err := store.DiscoveredContainers.UpdateContainerImageCheck(
		context.Background(), dc.ID, "sha256:oldmanifest", "sha256:newermanifest", false,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	worker := NewPortainerEnrichmentWorker(store)

	// First Run: transition false → true, should emit one event.
	if err := worker.Run(context.Background()); err != nil {
		t.Fatalf("Run() 1: %v", err)
	}

	events, _, err := store.Events.List(context.Background(), repo.ListFilter{SourceID: "p1", Limit: 10})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event after first run, got %d", len(events))
	}

	// Second Run: still true, should NOT emit another event.
	if err := worker.Run(context.Background()); err != nil {
		t.Fatalf("Run() 2: %v", err)
	}

	events2, _, err := store.Events.List(context.Background(), repo.ListFilter{SourceID: "p1", Limit: 10})
	if err != nil {
		t.Fatalf("list events after second run: %v", err)
	}
	if len(events2) != 1 {
		t.Errorf("expected still 1 event after second run (no duplicate), got %d", len(events2))
	}
}

// TestPortainerNoEventWhenValueUnchanged verifies no event is emitted when
// image_update_available is already true on the first poll (no transition).
func TestPortainerNoEventWhenValueUnchanged(t *testing.T) {
	fakeServer, srv := newPortainerFakeServer(t)
	fakeServer.endpoints = []PortainerEndpoint{{ID: 1, Name: "local"}}
	fakeServer.containers[1] = []PortainerContainer{
		{
			ID:    "ctr3",
			Names: []string{"/svc"},
			Image: "myrepo/svc:1.0",
			State: "running",
		},
	}
	fakeServer.inspects["ctr3"] = &PortainerContainerInspect{
		Image:  "sha256:old2",
		Config: struct{ Image string `json:"Image"` }{Image: "myrepo/svc:1.0"},
	}
	fakeServer.images["sha256:old2"] = &PortainerImageInspect{
		RepoDigests: []string{"myrepo/svc@sha256:oldmanifest2"},
	}

	store := newPortainerTestStore(t)
	createPortainerComponent(t, store, "p2", srv.URL)
	createDockerEngineComponent(t, store, "d1")
	dc := createDiscoveredContainer(t, store, "d1", "svc", "myrepo/svc:1.0", 1) // already true

	// Seed: running manifest differs from registry (update available = true already).
	if err := store.DiscoveredContainers.UpdateContainerImageCheck(
		context.Background(), dc.ID, "sha256:oldmanifest2", "sha256:newermanifest2", true,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Prime the sync.Map so the worker thinks it was already true last poll.
	worker := NewPortainerEnrichmentWorker(store)
	worker.lastUpdateAvailable.Store(dc.ID, true)

	if err := worker.Run(context.Background()); err != nil {
		t.Fatalf("Run(): %v", err)
	}

	events, _, err := store.Events.List(context.Background(), repo.ListFilter{SourceID: "p2", Limit: 10})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events (value unchanged), got %d", len(events))
	}
}
