package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/digitalcheffe/nora/internal/api"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

// newTopologyRouter wires a TopologyHandler onto a chi router using a real in-memory DB.
func newTopologyRouter(t *testing.T) http.Handler {
	t.Helper()
	db := newTestDB(t)
	ic := repo.NewInfraComponentRepo(db)
	de := repo.NewDockerEngineRepo(db)
	apps := repo.NewAppRepo(db)
	rollups := repo.NewResourceRollupRepo(db)
	h := api.NewTopologyHandler(ic, de, apps, rollups)
	r := chi.NewRouter()
	h.Routes(r)
	return r
}

// helpers

func createDockerEngine(t *testing.T, router http.Handler, name, socketType, socketPath, infraComponentID string) models.DockerEngine {
	t.Helper()
	payload := map[string]any{"name": name, "socket_type": socketType, "socket_path": socketPath}
	if infraComponentID != "" {
		payload["infra_component_id"] = infraComponentID
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/docker-engines", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createDockerEngine: expected 201 got %d: %s", rr.Code, rr.Body.String())
	}
	var e models.DockerEngine
	if err := json.NewDecoder(rr.Body).Decode(&e); err != nil {
		t.Fatalf("createDockerEngine decode: %v", err)
	}
	return e
}

// ---- DockerEngine CRUD ------------------------------------------------------

func TestCreateDockerEngine_HappyPath(t *testing.T) {
	router := newTopologyRouter(t)
	e := createDockerEngine(t, router, "docker-01", "local", "/var/run/docker.sock", "")
	if e.ID == "" {
		t.Error("expected non-empty ID")
	}
	if e.SocketType != "local" {
		t.Errorf("expected socket_type=local got %q", e.SocketType)
	}
}

func TestCreateDockerEngine_InvalidSocketType(t *testing.T) {
	router := newTopologyRouter(t)
	body, _ := json.Marshal(map[string]any{
		"name": "docker-01", "socket_type": "tcp", "socket_path": "/var/run/docker.sock",
	})
	req := httptest.NewRequest(http.MethodPost, "/docker-engines", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

func TestCreateDockerEngine_InvalidParentID(t *testing.T) {
	router := newTopologyRouter(t)
	body, _ := json.Marshal(map[string]any{
		"name": "docker-01", "socket_type": "local", "socket_path": "/var/run/docker.sock",
		"infra_component_id": "does-not-exist",
	})
	req := httptest.NewRequest(http.MethodPost, "/docker-engines", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 got %d", rr.Code)
	}
}

func TestGetDockerEngine_NotFound(t *testing.T) {
	router := newTopologyRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/docker-engines/does-not-exist", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}

func TestUpdateDockerEngine_HappyPath(t *testing.T) {
	router := newTopologyRouter(t)
	e := createDockerEngine(t, router, "docker-01", "local", "/var/run/docker.sock", "")
	body, _ := json.Marshal(map[string]any{"name": "docker-renamed"})
	req := httptest.NewRequest(http.MethodPut, "/docker-engines/"+e.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}
	var updated models.DockerEngine
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated.Name != "docker-renamed" {
		t.Errorf("expected updated name got %q", updated.Name)
	}
}

func TestUpdateDockerEngine_NotFound(t *testing.T) {
	router := newTopologyRouter(t)
	body, _ := json.Marshal(map[string]any{"name": "x"})
	req := httptest.NewRequest(http.MethodPut, "/docker-engines/does-not-exist", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}

func TestDeleteDockerEngine_HappyPath(t *testing.T) {
	router := newTopologyRouter(t)
	e := createDockerEngine(t, router, "docker-01", "local", "/var/run/docker.sock", "")
	req := httptest.NewRequest(http.MethodDelete, "/docker-engines/"+e.ID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 got %d", rr.Code)
	}
}

func TestDeleteDockerEngine_NotFound(t *testing.T) {
	router := newTopologyRouter(t)
	req := httptest.NewRequest(http.MethodDelete, "/docker-engines/does-not-exist", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}

// ---- GET /topology ----------------------------------------------------------

func TestGetTopology_EmptyChain(t *testing.T) {
	router := newTopologyRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/topology", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
	var result []any
	json.NewDecoder(rr.Body).Decode(&result)
	if len(result) != 0 {
		t.Errorf("expected empty array got %d items", len(result))
	}
}

func TestGetTopology_FullChain(t *testing.T) {
	db := newTestDB(t)
	ic := repo.NewInfraComponentRepo(db)
	de := repo.NewDockerEngineRepo(db)
	apps := repo.NewAppRepo(db)
	rollups := repo.NewResourceRollupRepo(db)

	// Use a combined router so both topology and infra_components are available.
	topoHandler := api.NewTopologyHandler(ic, de, apps, rollups)
	checks := repo.NewCheckRepo(db)
	tc := repo.NewTraefikComponentRepo(db)
	store := repo.NewStore(
		apps, repo.NewEventRepo(db), checks,
		repo.NewRollupRepo(db), repo.NewResourceReadingRepo(db), rollups,
		ic, de,
		repo.NewInfraRepo(db), repo.NewSettingsRepo(db), repo.NewMetricsRepo(db),
		repo.NewUserRepo(db), tc,
		repo.NewTraefikOverviewRepo(db), repo.NewTraefikServiceRepo(db),
		repo.NewDiscoveredContainerRepo(db), repo.NewDiscoveredRouteRepo(db), nil,
	)
	icHandler := api.NewInfraComponentHandler(ic, rollups, checks, tc, store)
	r := chi.NewRouter()
	topoHandler.Routes(r)
	icHandler.Routes(r)
	router := http.Handler(r)

	// Create a proxmox node (root).
	node := createInfraComponent(t, router, "proxmox-node1", "192.168.1.10", "proxmox_node", "")
	// Create a VM parented to the node.
	vm := createInfraComponent(t, router, "rocky-vm01", "192.168.1.50", "vm", node.ID)
	// Attach a docker engine to the VM.
	createDockerEngine(t, router, "docker-01", "local", "/var/run/docker.sock", vm.ID)

	req := httptest.NewRequest(http.MethodGet, "/topology", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}

	var chain []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Children []struct {
			ID            string `json:"id"`
			DockerEngines []struct {
				ID   string `json:"id"`
				Apps []any  `json:"apps"`
			} `json:"docker_engines"`
		} `json:"children"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&chain); err != nil {
		t.Fatalf("decode topology: %v", err)
	}
	if len(chain) != 1 {
		t.Fatalf("expected 1 root component got %d", len(chain))
	}
	if chain[0].ID != node.ID {
		t.Errorf("expected node id=%s got %s", node.ID, chain[0].ID)
	}
	if len(chain[0].Children) != 1 {
		t.Fatalf("expected 1 child got %d", len(chain[0].Children))
	}
	if len(chain[0].Children[0].DockerEngines) != 1 {
		t.Fatalf("expected 1 docker engine got %d", len(chain[0].Children[0].DockerEngines))
	}
	if len(chain[0].Children[0].DockerEngines[0].Apps) != 0 {
		t.Errorf("expected 0 apps got %d", len(chain[0].Children[0].DockerEngines[0].Apps))
	}
}
