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
	ph := repo.NewPhysicalHostRepo(db)
	vh := repo.NewVirtualHostRepo(db)
	de := repo.NewDockerEngineRepo(db)
	apps := repo.NewAppRepo(db)
	h := api.NewTopologyHandler(ph, vh, de, apps)
	r := chi.NewRouter()
	h.Routes(r)
	return r
}

// helpers

func createPhysicalHost(t *testing.T, router http.Handler, name, ip, hostType string) models.PhysicalHost {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"name": name, "ip": ip, "type": hostType})
	req := httptest.NewRequest(http.MethodPost, "/hosts/physical", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createPhysicalHost: expected 201 got %d: %s", rr.Code, rr.Body.String())
	}
	var h models.PhysicalHost
	if err := json.NewDecoder(rr.Body).Decode(&h); err != nil {
		t.Fatalf("createPhysicalHost decode: %v", err)
	}
	return h
}

func createVirtualHost(t *testing.T, router http.Handler, name, ip, hostType, physicalHostID string) models.VirtualHost {
	t.Helper()
	payload := map[string]any{"name": name, "ip": ip, "type": hostType}
	if physicalHostID != "" {
		payload["physical_host_id"] = physicalHostID
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/hosts/virtual", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("createVirtualHost: expected 201 got %d: %s", rr.Code, rr.Body.String())
	}
	var h models.VirtualHost
	if err := json.NewDecoder(rr.Body).Decode(&h); err != nil {
		t.Fatalf("createVirtualHost decode: %v", err)
	}
	return h
}

func createDockerEngine(t *testing.T, router http.Handler, name, socketType, socketPath, virtualHostID string) models.DockerEngine {
	t.Helper()
	payload := map[string]any{"name": name, "socket_type": socketType, "socket_path": socketPath}
	if virtualHostID != "" {
		payload["virtual_host_id"] = virtualHostID
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

// ---- PhysicalHost CRUD -------------------------------------------------------

func TestListPhysicalHosts_Empty(t *testing.T) {
	router := newTopologyRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/hosts/physical", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
	var resp struct {
		Data  []any `json:"data"`
		Total int   `json:"total"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Total != 0 {
		t.Errorf("expected total=0 got %d", resp.Total)
	}
}

func TestCreatePhysicalHost_HappyPath(t *testing.T) {
	router := newTopologyRouter(t)
	h := createPhysicalHost(t, router, "proxmox-node1", "192.168.1.10", "proxmox_node")
	if h.ID == "" {
		t.Error("expected non-empty ID")
	}
	if h.Name != "proxmox-node1" {
		t.Errorf("expected name=proxmox-node1 got %q", h.Name)
	}
}

func TestCreatePhysicalHost_InvalidType(t *testing.T) {
	router := newTopologyRouter(t)
	body, _ := json.Marshal(map[string]any{"name": "test", "ip": "1.2.3.4", "type": "unknown"})
	req := httptest.NewRequest(http.MethodPost, "/hosts/physical", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

func TestCreatePhysicalHost_MissingName(t *testing.T) {
	router := newTopologyRouter(t)
	body := bytes.NewBufferString(`{"ip":"1.2.3.4","type":"bare_metal"}`)
	req := httptest.NewRequest(http.MethodPost, "/hosts/physical", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

func TestGetPhysicalHost_HappyPath(t *testing.T) {
	router := newTopologyRouter(t)
	created := createPhysicalHost(t, router, "host1", "10.0.0.1", "bare_metal")
	req := httptest.NewRequest(http.MethodGet, "/hosts/physical/"+created.ID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
}

func TestGetPhysicalHost_NotFound(t *testing.T) {
	router := newTopologyRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/hosts/physical/does-not-exist", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}

func TestUpdatePhysicalHost_HappyPath(t *testing.T) {
	router := newTopologyRouter(t)
	created := createPhysicalHost(t, router, "host1", "10.0.0.1", "bare_metal")
	body, _ := json.Marshal(map[string]any{"name": "host1-renamed"})
	req := httptest.NewRequest(http.MethodPut, "/hosts/physical/"+created.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}
	var h models.PhysicalHost
	json.NewDecoder(rr.Body).Decode(&h)
	if h.Name != "host1-renamed" {
		t.Errorf("expected updated name got %q", h.Name)
	}
}

func TestUpdatePhysicalHost_NotFound(t *testing.T) {
	router := newTopologyRouter(t)
	body, _ := json.Marshal(map[string]any{"name": "x"})
	req := httptest.NewRequest(http.MethodPut, "/hosts/physical/does-not-exist", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}

func TestDeletePhysicalHost_HappyPath(t *testing.T) {
	router := newTopologyRouter(t)
	created := createPhysicalHost(t, router, "host1", "10.0.0.1", "bare_metal")
	req := httptest.NewRequest(http.MethodDelete, "/hosts/physical/"+created.ID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 got %d", rr.Code)
	}
}

func TestDeletePhysicalHost_NotFound(t *testing.T) {
	router := newTopologyRouter(t)
	req := httptest.NewRequest(http.MethodDelete, "/hosts/physical/does-not-exist", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}

// Deleting a physical host must SET NULL on virtual hosts (not cascade-delete).
func TestDeletePhysicalHost_DoesNotCascadeToVirtualHosts(t *testing.T) {
	router := newTopologyRouter(t)
	ph := createPhysicalHost(t, router, "phys", "10.0.0.1", "bare_metal")
	vh := createVirtualHost(t, router, "vm01", "10.0.0.2", "vm", ph.ID)

	// Delete the physical host.
	req := httptest.NewRequest(http.MethodDelete, "/hosts/physical/"+ph.ID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 got %d", rr.Code)
	}

	// Virtual host must still exist.
	req2 := httptest.NewRequest(http.MethodGet, "/hosts/virtual/"+vh.ID, nil)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("virtual host should still exist after physical host deleted, got %d", rr2.Code)
	}
}

// ---- VirtualHost CRUD -------------------------------------------------------

func TestCreateVirtualHost_HappyPath(t *testing.T) {
	router := newTopologyRouter(t)
	vh := createVirtualHost(t, router, "rocky-vm01", "192.168.1.50", "vm", "")
	if vh.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestCreateVirtualHost_InvalidType(t *testing.T) {
	router := newTopologyRouter(t)
	body, _ := json.Marshal(map[string]any{"name": "test", "ip": "1.2.3.4", "type": "kvm"})
	req := httptest.NewRequest(http.MethodPost, "/hosts/virtual", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

func TestCreateVirtualHost_InvalidParentID(t *testing.T) {
	router := newTopologyRouter(t)
	body, _ := json.Marshal(map[string]any{
		"name": "vm01", "ip": "1.2.3.4", "type": "vm",
		"physical_host_id": "does-not-exist",
	})
	req := httptest.NewRequest(http.MethodPost, "/hosts/virtual", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 got %d", rr.Code)
	}
}

func TestListVirtualHosts_IncludesDockerEngineIDs(t *testing.T) {
	router := newTopologyRouter(t)
	vh := createVirtualHost(t, router, "vm01", "10.0.0.1", "vm", "")
	createDockerEngine(t, router, "docker-01", "local", "/var/run/docker.sock", vh.ID)

	req := httptest.NewRequest(http.MethodGet, "/hosts/virtual", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
	var resp struct {
		Data []struct {
			ID            string   `json:"id"`
			DockerEngines []string `json:"docker_engines"`
		} `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 virtual host got %d", len(resp.Data))
	}
	if len(resp.Data[0].DockerEngines) != 1 {
		t.Errorf("expected 1 docker engine ID got %d", len(resp.Data[0].DockerEngines))
	}
}

func TestGetVirtualHost_NotFound(t *testing.T) {
	router := newTopologyRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/hosts/virtual/does-not-exist", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}

func TestDeleteVirtualHost_HappyPath(t *testing.T) {
	router := newTopologyRouter(t)
	vh := createVirtualHost(t, router, "vm01", "10.0.0.1", "vm", "")
	req := httptest.NewRequest(http.MethodDelete, "/hosts/virtual/"+vh.ID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 got %d", rr.Code)
	}
}

func TestDeleteVirtualHost_NotFound(t *testing.T) {
	router := newTopologyRouter(t)
	req := httptest.NewRequest(http.MethodDelete, "/hosts/virtual/does-not-exist", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
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
		"virtual_host_id": "does-not-exist",
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
	router := newTopologyRouter(t)

	ph := createPhysicalHost(t, router, "proxmox-node1", "192.168.1.10", "proxmox_node")
	vh := createVirtualHost(t, router, "rocky-vm01", "192.168.1.50", "vm", ph.ID)
	createDockerEngine(t, router, "docker-01", "local", "/var/run/docker.sock", vh.ID)

	req := httptest.NewRequest(http.MethodGet, "/topology", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}

	var chain []struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		VirtualHosts []struct {
			ID            string `json:"id"`
			DockerEngines []struct {
				ID   string `json:"id"`
				Apps []any  `json:"apps"`
			} `json:"docker_engines"`
		} `json:"virtual_hosts"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&chain); err != nil {
		t.Fatalf("decode topology: %v", err)
	}
	if len(chain) != 1 {
		t.Fatalf("expected 1 physical host got %d", len(chain))
	}
	if chain[0].ID != ph.ID {
		t.Errorf("expected physical host id=%s got %s", ph.ID, chain[0].ID)
	}
	if len(chain[0].VirtualHosts) != 1 {
		t.Fatalf("expected 1 virtual host got %d", len(chain[0].VirtualHosts))
	}
	if len(chain[0].VirtualHosts[0].DockerEngines) != 1 {
		t.Fatalf("expected 1 docker engine got %d", len(chain[0].VirtualHosts[0].DockerEngines))
	}
	if len(chain[0].VirtualHosts[0].DockerEngines[0].Apps) != 0 {
		t.Errorf("expected 0 apps got %d", len(chain[0].VirtualHosts[0].DockerEngines[0].Apps))
	}
}

func TestListPhysicalHosts_IncludesVirtualHostIDs(t *testing.T) {
	router := newTopologyRouter(t)
	ph := createPhysicalHost(t, router, "phys", "10.0.0.1", "bare_metal")
	createVirtualHost(t, router, "vm01", "10.0.0.2", "vm", ph.ID)

	req := httptest.NewRequest(http.MethodGet, "/hosts/physical", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	var resp struct {
		Data []struct {
			ID           string   `json:"id"`
			VirtualHosts []string `json:"virtual_hosts"`
		} `json:"data"`
	}
	json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 physical host got %d", len(resp.Data))
	}
	if len(resp.Data[0].VirtualHosts) != 1 {
		t.Errorf("expected 1 virtual host ID got %d", len(resp.Data[0].VirtualHosts))
	}
}
