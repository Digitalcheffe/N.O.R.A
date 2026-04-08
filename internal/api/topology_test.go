package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/digitalcheffe/nora/internal/api"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

// newTopologyRouter wires a TopologyHandler onto a chi router using a real in-memory DB.
func newTopologyRouter(t *testing.T) http.Handler {
	t.Helper()
	db := newTestDB(t)
	ic := repo.NewInfraComponentRepo(db)
	apps := repo.NewAppRepo(db)
	links := repo.NewComponentLinkRepo(db)
	h := api.NewTopologyHandler(ic, apps, links)
	r := chi.NewRouter()
	h.Routes(r)
	return r
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
	apps := repo.NewAppRepo(db)
	rollups := repo.NewResourceRollupRepo(db)
	links := repo.NewComponentLinkRepo(db)

	// Use a combined router so both topology and infra_components are available.
	topoHandler := api.NewTopologyHandler(ic, apps, links)
	checks := repo.NewCheckRepo(db)
	store := repo.NewStore(
		apps, repo.NewEventRepo(db), checks,
		repo.NewRollupRepo(db), repo.NewResourceReadingRepo(db), rollups,
		ic,
		repo.NewSettingsRepo(db), repo.NewMetricsRepo(db),
		repo.NewUserRepo(db),
		repo.NewDiscoveredContainerRepo(db), repo.NewDiscoveredRouteRepo(db), nil, nil, nil, nil, nil,
		links,
	)
	icHandler := api.NewInfraComponentHandler(ic, rollups, checks, repo.NewEventRepo(db), store)
	r := chi.NewRouter()
	topoHandler.Routes(r)
	icHandler.Routes(r)
	router := http.Handler(r)

	// Create a proxmox node (root).
	node := createInfraComponent(t, router, "proxmox-node1", "192.168.1.10", "proxmox_node", "")
	// Create a VM parented to the node.
	vm := createInfraComponent(t, router, "rocky-vm01", "192.168.1.50", "vm_other", node.ID)
	// Create a docker engine infra component parented to the VM.
	de := createInfraComponent(t, router, "docker-01", "", "docker_engine", vm.ID)

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
			ID       string `json:"id"`
			Children []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
				Apps []any  `json:"apps"`
			} `json:"children"`
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
		t.Fatalf("expected 1 vm child got %d", len(chain[0].Children))
	}
	if chain[0].Children[0].ID != vm.ID {
		t.Errorf("expected vm id=%s got %s", vm.ID, chain[0].Children[0].ID)
	}
	if len(chain[0].Children[0].Children) != 1 {
		t.Fatalf("expected 1 docker engine child got %d", len(chain[0].Children[0].Children))
	}
	if chain[0].Children[0].Children[0].ID != de.ID {
		t.Errorf("expected docker engine id=%s got %s", de.ID, chain[0].Children[0].Children[0].ID)
	}
	if chain[0].Children[0].Children[0].Type != "docker_engine" {
		t.Errorf("expected type docker_engine got %q", chain[0].Children[0].Children[0].Type)
	}
	if len(chain[0].Children[0].Children[0].Apps) != 0 {
		t.Errorf("expected 0 apps got %d", len(chain[0].Children[0].Children[0].Apps))
	}
}
