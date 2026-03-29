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

func newSynologyTestStore(t *testing.T) *repo.Store {
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

func createSynologyTestComponent(t *testing.T, store *repo.Store, id string) {
	t.Helper()
	c := &models.InfrastructureComponent{
		ID:               id,
		Name:             "syn-" + id,
		Type:             "synology",
		CollectionMethod: "synology_api",
		Enabled:          true,
		LastStatus:       "unknown",
		CreatedAt:        "2026-01-01T00:00:00Z",
	}
	if err := store.InfraComponents.Create(context.Background(), c); err != nil {
		t.Fatalf("create synology test component: %v", err)
	}
}

// synologyFakeServer handles DSM API requests.
type synologyFakeServer struct {
	sidToReturn     string
	loginShouldFail bool

	// expireFirstN causes the first N entry.cgi calls to return code 119.
	expireFirstN int
	callCount    int

	systemInfo synoSystemInfo
	volumes    []synoVolume
	disks      []synoDisk
}

func newSynologyFakeServer(t *testing.T) (*httptest.Server, *synologyFakeServer) {
	t.Helper()
	fs := &synologyFakeServer{
		sidToReturn: "test-session-id",
		systemInfo: synoSystemInfo{
			CPUUserLoad: 25.0,
			RAMTotal:    8192,
			RAMUsed:     4096,
		},
		volumes: []synoVolume{
			{VolPath: "/volume1", SizeTotalByte: "1000000000", SizeUsedByte: "500000000"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(fs.handle))
	t.Cleanup(srv.Close)
	return srv, fs
}

func (fs *synologyFakeServer) ok(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	b, _ := json.Marshal(map[string]interface{}{"success": true, "data": data})
	w.Write(b) //nolint:errcheck
}

func (fs *synologyFakeServer) apiError(w http.ResponseWriter, code int) {
	w.Header().Set("Content-Type", "application/json")
	b, _ := json.Marshal(map[string]interface{}{
		"success": false,
		"error":   map[string]interface{}{"code": code},
	})
	w.Write(b) //nolint:errcheck
}

func (fs *synologyFakeServer) handle(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	if r.URL.Path == "/webapi/auth.cgi" {
		switch q.Get("method") {
		case "login":
			if fs.loginShouldFail {
				fs.apiError(w, 400)
				return
			}
			fs.ok(w, map[string]string{"sid": fs.sidToReturn})
		case "logout":
			fs.ok(w, nil)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
		return
	}

	if r.URL.Path == "/webapi/entry.cgi" {
		// Simulate session expiry on the first N calls.
		if fs.expireFirstN > 0 && fs.callCount < fs.expireFirstN {
			fs.callCount++
			fs.apiError(w, 119)
			return
		}
		fs.callCount++

		switch q.Get("api") {
		case "SYNO.Core.System":
			fs.ok(w, fs.systemInfo)
		case "SYNO.Storage.CGI.Storage":
			fs.ok(w, map[string]interface{}{"volumes": fs.volumes})
		case "SYNO.Storage.CGI.DiskHealth":
			fs.ok(w, map[string]interface{}{"disks": fs.disks})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func makeSynologyCredentials(baseURL string) string {
	b, _ := json.Marshal(SynologyCredentials{
		BaseURL:   baseURL,
		Username:  "nora",
		Password:  "secret",
		VerifyTLS: true,
	})
	return string(b)
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestSynologyPoller_NewPoller_InvalidJSON(t *testing.T) {
	_, err := NewSynologyPoller("id1", "not-json")
	if err == nil {
		t.Error("expected error for invalid credentials JSON, got nil")
	}
}

func TestSynologyPoller_Poll_WritesResourceReadingsAndSetsOnline(t *testing.T) {
	srv, _ := newSynologyFakeServer(t)

	ctx := context.Background()
	store := newSynologyTestStore(t)
	compID := "syn-readings"
	createSynologyTestComponent(t, store, compID)

	poller, err := NewSynologyPoller(compID, makeSynologyCredentials(srv.URL))
	if err != nil {
		t.Fatalf("NewSynologyPoller: %v", err)
	}
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("Poll: %v", err)
	}

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
	for _, want := range []string{"cpu_percent", "mem_percent", "disk_percent_volume1"} {
		if !metrics[want] {
			t.Errorf("expected resource reading for metric %q, not found", want)
		}
	}
}

func TestSynologyPoller_Poll_CPUMemValues(t *testing.T) {
	srv, fs := newSynologyFakeServer(t)
	fs.systemInfo = synoSystemInfo{
		CPUUserLoad: 50.0,
		RAMTotal:    8192,
		RAMUsed:     4096, // 50%
	}
	fs.volumes = []synoVolume{
		{VolPath: "/volume1", SizeTotalByte: "1000000000", SizeUsedByte: "500000000"}, // 50%
	}

	ctx := context.Background()
	store := newSynologyTestStore(t)
	compID := "syn-values"
	createSynologyTestComponent(t, store, compID)

	poller, _ := NewSynologyPoller(compID, makeSynologyCredentials(srv.URL))
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
		if a.Avg < 49 || a.Avg > 51 {
			t.Errorf("metric %s: avg=%v, want ~50", a.Metric, a.Avg)
		}
	}
}

func TestSynologyPoller_Poll_MultipleVolumes(t *testing.T) {
	srv, fs := newSynologyFakeServer(t)
	fs.volumes = []synoVolume{
		{VolPath: "/volume1", SizeTotalByte: "1000000000", SizeUsedByte: "250000000"},  // 25%
		{VolPath: "/volume2", SizeTotalByte: "2000000000", SizeUsedByte: "1000000000"}, // 50%
	}

	ctx := context.Background()
	store := newSynologyTestStore(t)
	compID := "syn-multivol"
	createSynologyTestComponent(t, store, compID)

	poller, _ := NewSynologyPoller(compID, makeSynologyCredentials(srv.URL))
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("Poll: %v", err)
	}

	from := time.Now().UTC().Add(-time.Minute)
	to := time.Now().UTC().Add(time.Minute)
	aggs, _ := store.ResourceRollups.AggregateReadings(ctx, from, to)

	metrics := make(map[string]float64)
	for _, a := range aggs {
		if a.SourceID == compID {
			metrics[a.Metric] = a.Avg
		}
	}
	if _, ok := metrics["disk_percent_volume1"]; !ok {
		t.Error("expected disk_percent_volume1 reading")
	}
	if _, ok := metrics["disk_percent_volume2"]; !ok {
		t.Error("expected disk_percent_volume2 reading")
	}
}

func TestSynologyPoller_Poll_DiskWarningFiresWarnEvent(t *testing.T) {
	srv, fs := newSynologyFakeServer(t)
	fs.disks = []synoDisk{{ID: "sda", Status: "warning"}}

	ctx := context.Background()
	store := newSynologyTestStore(t)
	compID := "syn-disk-warn"
	createSynologyTestComponent(t, store, compID)

	poller, _ := NewSynologyPoller(compID, makeSynologyCredentials(srv.URL))
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("Poll: %v", err)
	}

	comp, _ := store.InfraComponents.Get(ctx, compID)
	if comp.LastStatus != "degraded" {
		t.Errorf("last_status: got %q, want \"degraded\"", comp.LastStatus)
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
		if ev.Severity == "warn" && ev.DisplayText == "Synology disk sda status: warning" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warn event for sda warning; events: %v", events)
	}
}

func TestSynologyPoller_Poll_DiskCriticalFiresErrorEvent(t *testing.T) {
	srv, fs := newSynologyFakeServer(t)
	fs.disks = []synoDisk{{ID: "sdb", Status: "critical"}}

	ctx := context.Background()
	store := newSynologyTestStore(t)
	compID := "syn-disk-crit"
	createSynologyTestComponent(t, store, compID)

	poller, _ := NewSynologyPoller(compID, makeSynologyCredentials(srv.URL))
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("Poll: %v", err)
	}

	events, _, _ := store.Events.List(ctx, repo.ListFilter{Limit: 50})
	found := false
	for _, ev := range events {
		if ev.Severity == "error" && ev.DisplayText == "Synology disk sdb status: critical" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error event for sdb critical; events: %v", events)
	}
}

func TestSynologyPoller_Poll_DiskFailingFiresErrorEvent(t *testing.T) {
	srv, fs := newSynologyFakeServer(t)
	fs.disks = []synoDisk{{ID: "sdc", Status: "failing"}}

	ctx := context.Background()
	store := newSynologyTestStore(t)
	compID := "syn-disk-fail"
	createSynologyTestComponent(t, store, compID)

	poller, _ := NewSynologyPoller(compID, makeSynologyCredentials(srv.URL))
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("Poll: %v", err)
	}

	events, _, _ := store.Events.List(ctx, repo.ListFilter{Limit: 50})
	found := false
	for _, ev := range events {
		if ev.Severity == "error" && ev.DisplayText == "Synology disk sdc status: failing" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error event for sdc failing; events: %v", events)
	}
}

func TestSynologyPoller_Poll_AllDisksNormal_NoEvent(t *testing.T) {
	srv, fs := newSynologyFakeServer(t)
	fs.disks = []synoDisk{
		{ID: "sda", Status: "normal"},
		{ID: "sdb", Status: "normal"},
	}

	ctx := context.Background()
	store := newSynologyTestStore(t)
	compID := "syn-disk-ok"
	createSynologyTestComponent(t, store, compID)

	poller, _ := NewSynologyPoller(compID, makeSynologyCredentials(srv.URL))
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("Poll: %v", err)
	}

	_, total, _ := store.Events.List(ctx, repo.ListFilter{Limit: 50})
	if total != 0 {
		t.Errorf("expected 0 events for normal disks, got %d", total)
	}

	comp, _ := store.InfraComponents.Get(ctx, compID)
	if comp.LastStatus != "online" {
		t.Errorf("last_status: got %q, want \"online\"", comp.LastStatus)
	}
}

func TestSynologyPoller_Poll_SessionExpiryTriggersReauth(t *testing.T) {
	srv, fs := newSynologyFakeServer(t)
	// First entry.cgi call returns session-expired; poller should re-auth and retry.
	fs.expireFirstN = 1

	ctx := context.Background()
	store := newSynologyTestStore(t)
	compID := "syn-reauth"
	createSynologyTestComponent(t, store, compID)

	poller, _ := NewSynologyPoller(compID, makeSynologyCredentials(srv.URL))
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("Poll after session expiry: %v", err)
	}

	comp, _ := store.InfraComponents.Get(ctx, compID)
	if comp.LastStatus != "online" {
		t.Errorf("last_status after reauth: got %q, want \"online\"", comp.LastStatus)
	}
}

func TestSynologyPoller_Poll_SessionReusedAcrossCycles(t *testing.T) {
	loginCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == "/webapi/auth.cgi" && q.Get("method") == "login" {
			loginCount++
			b, _ := json.Marshal(map[string]interface{}{
				"success": true,
				"data":    map[string]string{"sid": "reused-sid"},
			})
			w.Write(b) //nolint:errcheck
			return
		}

		if r.URL.Path == "/webapi/entry.cgi" {
			var data interface{}
			switch q.Get("api") {
			case "SYNO.Core.System":
				data = synoSystemInfo{CPUUserLoad: 10, RAMTotal: 1024, RAMUsed: 512}
			case "SYNO.Storage.CGI.Storage":
				data = map[string]interface{}{"volumes": []interface{}{}}
			case "SYNO.Storage.CGI.DiskHealth":
				data = map[string]interface{}{"disks": []interface{}{}}
			}
			b, _ := json.Marshal(map[string]interface{}{"success": true, "data": data})
			w.Write(b) //nolint:errcheck
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	ctx := context.Background()
	store := newSynologyTestStore(t)
	compID := "syn-session-reuse"
	createSynologyTestComponent(t, store, compID)

	poller, _ := NewSynologyPoller(compID, makeSynologyCredentials(srv.URL))

	// Two poll cycles — login should only be called once.
	for i := 0; i < 2; i++ {
		if err := poller.Poll(ctx, store); err != nil {
			t.Fatalf("Poll cycle %d: %v", i+1, err)
		}
	}

	if loginCount != 1 {
		t.Errorf("login called %d times across 2 poll cycles, want 1", loginCount)
	}
}

func TestSynologyPoller_Poll_LoginFailure_ReturnsError(t *testing.T) {
	srv, fs := newSynologyFakeServer(t)
	fs.loginShouldFail = true

	ctx := context.Background()
	store := newSynologyTestStore(t)
	compID := "syn-login-fail"
	createSynologyTestComponent(t, store, compID)

	poller, _ := NewSynologyPoller(compID, makeSynologyCredentials(srv.URL))
	if err := poller.Poll(ctx, store); err == nil {
		t.Error("expected error when login fails, got nil")
	}
}

func TestSynologyPoller_Poll_ConnectionRefused_ReturnsError(t *testing.T) {
	ctx := context.Background()
	store := newSynologyTestStore(t)
	compID := "syn-offline"
	createSynologyTestComponent(t, store, compID)

	poller, err := NewSynologyPoller(compID, makeSynologyCredentials("http://127.0.0.1:1"))
	if err != nil {
		t.Fatalf("NewSynologyPoller: %v", err)
	}
	if err := poller.Poll(ctx, store); err == nil {
		t.Error("expected error for unreachable host, got nil")
	}
}
