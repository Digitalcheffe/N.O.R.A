package infra

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/migrations"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func newProxmoxTestStore(t *testing.T) *repo.Store {
	t.Helper()
	cfg := &config.Config{DBPath: ":memory:", DevMode: true}
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
		repo.NewDiscoveredContainerRepo(db),
		repo.NewDiscoveredRouteRepo(db),
		nil,
	)
}

// createTestComponent inserts a minimal proxmox_node component with the given ID.
func createTestComponent(t *testing.T, store *repo.Store, id string) {
	t.Helper()
	c := &models.InfrastructureComponent{
		ID:               id,
		Name:             "test-" + id,
		Type:             "proxmox_node",
		CollectionMethod: "proxmox_api",
		Enabled:          true,
		LastStatus:       "unknown",
		CreatedAt:        "2026-01-01T00:00:00Z",
	}
	if err := store.InfraComponents.Create(context.Background(), c); err != nil {
		t.Fatalf("create test component: %v", err)
	}
}

// proxmoxFakeServer builds an httptest.Server that responds with the supplied
// fixture data, keyed by node name.
type proxmoxFakeServer struct {
	nodes         []proxmoxNode
	statusByNode  map[string]proxmoxNodeStatus
	storageByNode map[string][]proxmoxStorage
	qemuByNode    map[string][]proxmoxVM
	lxcByNode     map[string][]proxmoxVM
}

func newFakeServer(t *testing.T) (*httptest.Server, *proxmoxFakeServer) {
	t.Helper()
	fs := &proxmoxFakeServer{
		statusByNode:  make(map[string]proxmoxNodeStatus),
		storageByNode: make(map[string][]proxmoxStorage),
		qemuByNode:    make(map[string][]proxmoxVM),
		lxcByNode:     make(map[string][]proxmoxVM),
	}
	srv := httptest.NewServer(http.HandlerFunc(fs.handle))
	t.Cleanup(srv.Close)
	return srv, fs
}

func (fs *proxmoxFakeServer) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	respond := func(v any) {
		b, _ := json.Marshal(map[string]any{"data": v})
		w.Write(b) //nolint:errcheck
	}

	if r.URL.Path == "/api2/json/nodes" {
		respond(fs.nodes)
		return
	}
	for _, n := range fs.nodes {
		base := "/api2/json/nodes/" + n.Node
		switch r.URL.Path {
		case base + "/status":
			respond(fs.statusByNode[n.Node])
			return
		case base + "/storage":
			respond(fs.storageByNode[n.Node])
			return
		case base + "/qemu":
			respond(fs.qemuByNode[n.Node])
			return
		case base + "/lxc":
			respond(fs.lxcByNode[n.Node])
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

func makeCredentials(baseURL string) string {
	b, _ := json.Marshal(ProxmoxCredentials{
		BaseURL:     baseURL,
		TokenID:     "nora@pam!token",
		TokenSecret: "secret",
		VerifyTLS:   true,
	})
	return string(b)
}

func defaultNodeStatus() proxmoxNodeStatus {
	var ns proxmoxNodeStatus
	ns.CPU = 0.25
	ns.Memory.Used = 2e9
	ns.Memory.Total = 8e9
	return ns
}

func defaultStorage() []proxmoxStorage {
	return []proxmoxStorage{{Storage: "local", Used: 50e9, Total: 200e9, Active: 1}}
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestProxmoxPoller_NewPoller_InvalidJSON(t *testing.T) {
	_, err := NewProxmoxPoller("id1", "not-json")
	if err == nil {
		t.Error("expected error for invalid credentials JSON, got nil")
	}
}

func TestProxmoxPoller_Poll_WritesResourceReadingsAndSetsOnline(t *testing.T) {
	srv, fs := newFakeServer(t)
	fs.nodes = []proxmoxNode{{Node: "pve", Status: "online"}}
	fs.statusByNode["pve"] = defaultNodeStatus()
	fs.storageByNode["pve"] = defaultStorage()

	ctx := context.Background()
	store := newProxmoxTestStore(t)
	compID := "comp-readings"
	createTestComponent(t, store, compID)

	poller, err := NewProxmoxPoller(compID, makeCredentials(srv.URL))
	if err != nil {
		t.Fatalf("NewProxmoxPoller: %v", err)
	}
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("Poll: %v", err)
	}

	// Component should now be "online" with last_polled_at set.
	comp, err := store.InfraComponents.Get(ctx, compID)
	if err != nil {
		t.Fatalf("Get component: %v", err)
	}
	if comp.LastStatus != "online" {
		t.Errorf("last_status: got %q, want %q", comp.LastStatus, "online")
	}
	if comp.LastPolledAt == nil {
		t.Error("last_polled_at should be set after a successful poll")
	}

	// Verify resource_readings via aggregation: 3 metrics (cpu, mem, disk) expected.
	from := time.Now().UTC().Add(-time.Minute)
	to := time.Now().UTC().Add(time.Minute)
	aggs, err := store.ResourceRollups.AggregateReadings(ctx, from, to)
	if err != nil {
		t.Fatalf("AggregateReadings: %v", err)
	}
	metrics := make(map[string]bool)
	for _, a := range aggs {
		if a.SourceID == compID {
			metrics[a.Metric] = true
		}
	}
	for _, want := range []string{"cpu_percent", "mem_percent", "disk_percent"} {
		if !metrics[want] {
			t.Errorf("expected resource reading for metric %q, not found", want)
		}
	}
}

func TestProxmoxPoller_Poll_CPUMemValues(t *testing.T) {
	srv, fs := newFakeServer(t)
	fs.nodes = []proxmoxNode{{Node: "pve", Status: "online"}}

	var ns proxmoxNodeStatus
	ns.CPU = 0.50            // 50%
	ns.Memory.Used = 4e9     // 4 GiB
	ns.Memory.Total = 8e9    // 8 GiB → 50%
	fs.statusByNode["pve"] = ns
	fs.storageByNode["pve"] = []proxmoxStorage{{Storage: "local", Used: 100e9, Total: 200e9, Active: 1}} // 50%

	ctx := context.Background()
	store := newProxmoxTestStore(t)
	compID := "comp-values"
	createTestComponent(t, store, compID)

	poller, _ := NewProxmoxPoller(compID, makeCredentials(srv.URL))
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("Poll: %v", err)
	}

	from := time.Now().UTC().Add(-time.Minute)
	to := time.Now().UTC().Add(time.Minute)
	aggs, _ := store.ResourceRollups.AggregateReadings(ctx, from, to)

	for _, a := range aggs {
		if a.SourceID != compID {
			continue
		}
		// All three metrics should be ~50.
		if a.Avg < 49 || a.Avg > 51 {
			t.Errorf("metric %s: avg=%v, want ~50", a.Metric, a.Avg)
		}
	}
}

func TestProxmoxPoller_Poll_VMStoppedFiresWarnEvent(t *testing.T) {
	srv, fs := newFakeServer(t)
	fs.nodes = []proxmoxNode{{Node: "pve", Status: "online"}}
	fs.statusByNode["pve"] = defaultNodeStatus()
	fs.storageByNode["pve"] = defaultStorage()
	fs.qemuByNode["pve"] = []proxmoxVM{{VMID: 100, Name: "ubuntu", Status: "running"}}

	ctx := context.Background()
	store := newProxmoxTestStore(t)
	compID := "comp-vm-stop"
	createTestComponent(t, store, compID)

	poller, _ := NewProxmoxPoller(compID, makeCredentials(srv.URL))

	// First poll: seeds state, no event.
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("first Poll: %v", err)
	}

	// VM transitions to stopped.
	fs.qemuByNode["pve"] = []proxmoxVM{{VMID: 100, Name: "ubuntu", Status: "stopped"}}

	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("second Poll: %v", err)
	}

	events, total, err := store.Events.List(ctx, repo.ListFilter{Limit: 50})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if total == 0 {
		t.Fatal("expected at least one event, got none")
	}
	found := false
	for _, ev := range events {
		if ev.Severity == "warn" && ev.DisplayText == "VM ubuntu is now stopped on pve" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warn event for VM ubuntu stopped; events: %v", events)
	}
}

func TestProxmoxPoller_Poll_VMStartedFiresInfoEvent(t *testing.T) {
	srv, fs := newFakeServer(t)
	fs.nodes = []proxmoxNode{{Node: "pve", Status: "online"}}
	fs.statusByNode["pve"] = defaultNodeStatus()
	fs.storageByNode["pve"] = defaultStorage()
	fs.qemuByNode["pve"] = []proxmoxVM{{VMID: 101, Name: "win11", Status: "stopped"}}

	ctx := context.Background()
	store := newProxmoxTestStore(t)
	compID := "comp-vm-start"
	createTestComponent(t, store, compID)

	poller, _ := NewProxmoxPoller(compID, makeCredentials(srv.URL))
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("first Poll: %v", err)
	}

	fs.qemuByNode["pve"] = []proxmoxVM{{VMID: 101, Name: "win11", Status: "running"}}
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("second Poll: %v", err)
	}

	events, _, _ := store.Events.List(ctx, repo.ListFilter{Limit: 50})
	found := false
	for _, ev := range events {
		if ev.Severity == "info" && ev.DisplayText == "VM win11 is now running on pve" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected info event for VM win11 started; events: %v", events)
	}
}

func TestProxmoxPoller_Poll_LXCStateChangeFiresEvent(t *testing.T) {
	srv, fs := newFakeServer(t)
	fs.nodes = []proxmoxNode{{Node: "pve", Status: "online"}}
	fs.statusByNode["pve"] = defaultNodeStatus()
	fs.storageByNode["pve"] = defaultStorage()
	fs.lxcByNode["pve"] = []proxmoxVM{{VMID: 200, Name: "debian-ct", Status: "stopped"}}

	ctx := context.Background()
	store := newProxmoxTestStore(t)
	compID := "comp-lxc"
	createTestComponent(t, store, compID)

	poller, _ := NewProxmoxPoller(compID, makeCredentials(srv.URL))
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("first Poll: %v", err)
	}

	fs.lxcByNode["pve"] = []proxmoxVM{{VMID: 200, Name: "debian-ct", Status: "running"}}
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("second Poll: %v", err)
	}

	events, _, _ := store.Events.List(ctx, repo.ListFilter{Limit: 50})
	found := false
	for _, ev := range events {
		if ev.Severity == "info" && ev.DisplayText == "LXC debian-ct is now running on pve" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected info event for LXC debian-ct started; events: %v", events)
	}
}

func TestProxmoxPoller_Poll_NoStateChangeNoEvent(t *testing.T) {
	srv, fs := newFakeServer(t)
	fs.nodes = []proxmoxNode{{Node: "pve", Status: "online"}}
	fs.statusByNode["pve"] = defaultNodeStatus()
	fs.storageByNode["pve"] = defaultStorage()
	fs.qemuByNode["pve"] = []proxmoxVM{{VMID: 100, Name: "ubuntu", Status: "running"}}

	ctx := context.Background()
	store := newProxmoxTestStore(t)
	compID := "comp-no-change"
	createTestComponent(t, store, compID)

	poller, _ := NewProxmoxPoller(compID, makeCredentials(srv.URL))
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("first Poll: %v", err)
	}
	// State unchanged on second poll.
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("second Poll: %v", err)
	}

	_, total, _ := store.Events.List(ctx, repo.ListFilter{Limit: 50})
	if total != 0 {
		t.Errorf("expected 0 events for no state change, got %d", total)
	}
}

func TestProxmoxPoller_Poll_ConnectionRefused_ReturnsError(t *testing.T) {
	creds := makeCredentials("http://127.0.0.1:1") // nothing listening

	ctx := context.Background()
	store := newProxmoxTestStore(t)
	compID := "comp-offline"
	createTestComponent(t, store, compID)

	poller, err := NewProxmoxPoller(compID, creds)
	if err != nil {
		t.Fatalf("NewProxmoxPoller: %v", err)
	}

	pollErr := poller.Poll(ctx, store)
	if pollErr == nil {
		t.Error("expected error for unreachable host, got nil")
	}
	// Process should not have panicked. If we reach here, it didn't.
}

func TestProxmoxPoller_AuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[]}`)) //nolint:errcheck
	}))
	defer srv.Close()

	creds, _ := json.Marshal(ProxmoxCredentials{
		BaseURL:     srv.URL,
		TokenID:     "nora@pam!mytoken",
		TokenSecret: "abc123",
		VerifyTLS:   true,
	})

	ctx := context.Background()
	store := newProxmoxTestStore(t)
	createTestComponent(t, store, "id-auth")

	poller, err := NewProxmoxPoller("id-auth", string(creds))
	if err != nil {
		t.Fatalf("NewProxmoxPoller: %v", err)
	}

	// Poll — returns nil because there are no nodes; auth header is still captured.
	_ = poller.Poll(ctx, store)

	want := "PVEAPIToken=nora@pam!mytoken=abc123"
	if gotAuth != want {
		t.Errorf("Authorization header: got %q, want %q", gotAuth, want)
	}
}
