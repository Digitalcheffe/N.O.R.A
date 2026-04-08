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
		repo.NewSettingsRepo(db),
		repo.NewMetricsRepo(db),
		repo.NewUserRepo(db),
		repo.NewDiscoveredContainerRepo(db),
		repo.NewDiscoveredRouteRepo(db),
		nil,
		nil,
		nil,
		nil,
		nil,
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

// synologyFakeServer handles DSM API requests:
// - Login/Logout: GET  /webapi/auth.cgi  api=SYNO.API.Auth
// - System info:  GET  /webapi/entry.cgi api=SYNO.Core.System method=info
// - Utilization:  GET  /webapi/entry.cgi api=SYNO.Core.System.Utilization method=get
// - Volumes:      GET  /webapi/entry.cgi api=SYNO.Core.System method=info type=storage
// - Disks:        GET  /webapi/entry.cgi api=SYNO.Storage.CGI.Storage method=load_info
// - Upgrade:      GET  /webapi/entry.cgi api=SYNO.Core.Upgrade method=check
type synologyFakeServer struct {
	sidToReturn     string
	loginShouldFail bool

	// expireFirstN causes the first N entry.cgi non-auth calls to return code 119.
	expireFirstN int
	callCount    int

	systemInfo  synoCoreSystemInfo
	utilization synoUtilization
	volumes     []synoVolumeInfo
	disks       []synoStorageDisk
	upgrade     synoUpgradeData
}

func newSynologyFakeServer(t *testing.T) (*httptest.Server, *synologyFakeServer) {
	t.Helper()
	fs := &synologyFakeServer{
		sidToReturn: "test-session-id",
		systemInfo: synoCoreSystemInfo{
			Model:       "DS920+",
			FirmwareVer: "7.2.1-69057",
			HostName:    "synology",
			UpTime:      "86400",
			Temperature: 38,
		},
		utilization: synoUtilization{
			CPU:    synoUtilCPU{UserLoad: 25.0},
			Memory: synoUtilMemory{RealUsage: 50.0, RealTotal: 8192, AvailReal: 4096},
		},
		volumes: []synoVolumeInfo{
			{VolPath: "/volume1", Status: "normal", TotalSize: "1000000000", UsedSize: "500000000"},
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

	apiName := q.Get("api")
	method := q.Get("method")

	// Auth calls (login/logout) go to auth.cgi.
	if r.URL.Path == "/webapi/auth.cgi" {
		switch method {
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

	if r.URL.Path != "/webapi/entry.cgi" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Simulate session expiry on the first N entry.cgi calls.
	if fs.expireFirstN > 0 && fs.callCount < fs.expireFirstN {
		fs.callCount++
		fs.apiError(w, 119)
		return
	}
	fs.callCount++

	switch apiName {
	case "SYNO.Core.System":
		if q.Get("type") == "storage" {
			fs.ok(w, map[string]interface{}{"vol_info": fs.volumes})
		} else {
			fs.ok(w, fs.systemInfo)
		}
	case "SYNO.Core.System.Utilization":
		fs.ok(w, fs.utilization)
	case "SYNO.Storage.CGI.Storage":
		fs.ok(w, map[string]interface{}{"disks": fs.disks})
	case "SYNO.Core.Upgrade":
		fs.ok(w, fs.upgrade)
	default:
		// Unknown API — return empty success so the poller treats it as non-fatal.
		fs.ok(w, map[string]interface{}{})
	}
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
	fs.utilization = synoUtilization{
		CPU:    synoUtilCPU{UserLoad: 50.0},
		Memory: synoUtilMemory{RealUsage: 50.0, RealTotal: 8192, AvailReal: 4096},
	}
	fs.volumes = []synoVolumeInfo{
		{VolPath: "/volume1", Status: "normal", TotalSize: "1000000000", UsedSize: "500000000"},
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
		if a.Metric == "cpu_percent" || a.Metric == "mem_percent" {
			if a.Avg < 49 || a.Avg > 51 {
				t.Errorf("metric %s: avg=%v, want ~50", a.Metric, a.Avg)
			}
		}
	}
}

func TestSynologyPoller_Poll_MultipleVolumes(t *testing.T) {
	srv, fs := newSynologyFakeServer(t)
	fs.volumes = []synoVolumeInfo{
		{VolPath: "/volume1", Status: "normal", TotalSize: "1000000000", UsedSize: "250000000"},
		{VolPath: "/volume2", Status: "normal", TotalSize: "2000000000", UsedSize: "1000000000"},
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
	fs.disks = []synoStorageDisk{{Slot: 1, Model: "WD Red", Status: "warning"}}

	ctx := context.Background()
	store := newSynologyTestStore(t)
	compID := "syn-disk-warn"
	createSynologyTestComponent(t, store, compID)

	poller, _ := NewSynologyPoller(compID, makeSynologyCredentials(srv.URL))
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("Poll: %v", err)
	}

	// A "warning" disk no longer degrades the component — only "critical" does.
	comp, _ := store.InfraComponents.Get(ctx, compID)
	if comp.LastStatus != "online" {
		t.Errorf("last_status: got %q, want \"online\" (warning disk is advisory, not degrading)", comp.LastStatus)
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
		if ev.Level == "warn" && ev.Title == "Disk 1 (WD Red) warning" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warn event for disk 1 warning; events: %v", events)
	}
}

func TestSynologyPoller_Poll_DiskCriticalFiresErrorEvent(t *testing.T) {
	srv, fs := newSynologyFakeServer(t)
	fs.disks = []synoStorageDisk{{Slot: 2, Model: "WD Red", Status: "critical"}}

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
		if ev.Level == "error" && ev.Title == "Disk 2 (WD Red) critical" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error event for disk 2 critical; events: %v", events)
	}
}

func TestSynologyPoller_Poll_AllDisksNormal_NoEvent(t *testing.T) {
	srv, fs := newSynologyFakeServer(t)
	fs.disks = []synoStorageDisk{
		{Slot: 1, Model: "WD Red", Status: "normal"},
		{Slot: 2, Model: "WD Red", Status: "normal"},
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

func TestSynologyPoller_Poll_DiskStatusOnlyOnTransition(t *testing.T) {
	srv, fs := newSynologyFakeServer(t)
	fs.disks = []synoStorageDisk{{Slot: 1, Model: "WD Red", Status: "warning"}}

	ctx := context.Background()
	store := newSynologyTestStore(t)
	compID := "syn-disk-transition"
	createSynologyTestComponent(t, store, compID)

	poller, _ := NewSynologyPoller(compID, makeSynologyCredentials(srv.URL))

	// First poll — status transition from "" to "warning" should fire one event.
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("Poll 1: %v", err)
	}
	_, total1, _ := store.Events.List(ctx, repo.ListFilter{Limit: 50})
	if total1 != 1 {
		t.Errorf("after first poll: expected 1 event, got %d", total1)
	}

	// Second poll — same status, no new event.
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("Poll 2: %v", err)
	}
	_, total2, _ := store.Events.List(ctx, repo.ListFilter{Limit: 50})
	if total2 != 1 {
		t.Errorf("after second poll (same status): expected still 1 event, got %d", total2)
	}
}

func TestSynologyPoller_Poll_SessionExpiryTriggersReauth(t *testing.T) {
	srv, fs := newSynologyFakeServer(t)
	// First entry.cgi non-auth call returns session-expired; poller should re-auth and retry.
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

		if r.URL.Path == "/webapi/auth.cgi" {
			if q.Get("method") == "login" {
				loginCount++
				b, _ := json.Marshal(map[string]interface{}{
					"success": true,
					"data":    map[string]string{"sid": "reused-sid"},
				})
				w.Write(b) //nolint:errcheck
			} else {
				// logout and any other auth calls
				b, _ := json.Marshal(map[string]interface{}{"success": true, "data": nil})
				w.Write(b) //nolint:errcheck
			}
			return
		}

		if r.URL.Path != "/webapi/entry.cgi" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// All other calls (entry.cgi) succeed with minimal data.
		var data interface{}
		switch q.Get("api") {
		case "SYNO.Core.System":
			if q.Get("type") == "storage" {
				data = map[string]interface{}{"vol_info": []interface{}{}}
			} else {
				data = synoCoreSystemInfo{Model: "DS920+"}
			}
		case "SYNO.Core.System.Utilization":
			data = synoUtilization{}
		case "SYNO.Storage.CGI.Storage":
			data = map[string]interface{}{"disks": []interface{}{}}
		case "SYNO.Core.Upgrade":
			data = synoUpgradeData{}
		default:
			data = map[string]interface{}{}
		}
		b, _ := json.Marshal(map[string]interface{}{"success": true, "data": data})
		w.Write(b) //nolint:errcheck
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

func TestSynologyPoller_Poll_VolumeStatusChangeFiresEvent(t *testing.T) {
	srv, fs := newSynologyFakeServer(t)
	fs.volumes = []synoVolumeInfo{
		{VolPath: "/volume1", Status: "normal", TotalSize: "1000000000", UsedSize: "500000000"},
	}

	ctx := context.Background()
	store := newSynologyTestStore(t)
	compID := "syn-vol-event"
	createSynologyTestComponent(t, store, compID)

	poller, _ := NewSynologyPoller(compID, makeSynologyCredentials(srv.URL))

	// First poll with normal status — no event.
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("Poll 1: %v", err)
	}
	_, total1, _ := store.Events.List(ctx, repo.ListFilter{Limit: 50})
	if total1 != 0 {
		t.Errorf("expected 0 events after normal poll, got %d", total1)
	}

	// Transition to degraded — event should fire.
	fs.volumes[0].Status = "degraded"
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("Poll 2: %v", err)
	}
	events, _, _ := store.Events.List(ctx, repo.ListFilter{Limit: 50})
	found := false
	for _, ev := range events {
		if ev.Level == "warn" && ev.Title == "Volume /volume1 degraded" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warn event for /volume1 degraded; events: %v", events)
	}
}

func TestSynologyPoller_Poll_DSMUpdateFiresEvent(t *testing.T) {
	srv, fs := newSynologyFakeServer(t)
	fs.upgrade = synoUpgradeData{Available: true, Version: "7.3.0-69999"}

	ctx := context.Background()
	store := newSynologyTestStore(t)
	compID := "syn-update"
	createSynologyTestComponent(t, store, compID)

	poller, _ := NewSynologyPoller(compID, makeSynologyCredentials(srv.URL))
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("Poll: %v", err)
	}

	events, _, _ := store.Events.List(ctx, repo.ListFilter{Limit: 50})
	found := false
	for _, ev := range events {
		if ev.Level == "info" && ev.Title == "DSM update available: 7.3.0-69999" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected info event for DSM update; events: %v", events)
	}
}

func TestSynologyPoller_Poll_MetaStored(t *testing.T) {
	srv, _ := newSynologyFakeServer(t)

	ctx := context.Background()
	store := newSynologyTestStore(t)
	compID := "syn-meta"
	createSynologyTestComponent(t, store, compID)

	poller, _ := NewSynologyPoller(compID, makeSynologyCredentials(srv.URL))
	if err := poller.Poll(ctx, store); err != nil {
		t.Fatalf("Poll: %v", err)
	}

	comp, _ := store.InfraComponents.Get(ctx, compID)
	if comp.Meta == nil || *comp.Meta == "" {
		t.Fatal("synology_meta should be non-empty after a successful poll")
	}

	var meta SynologyMeta
	if err := json.Unmarshal([]byte(*comp.Meta), &meta); err != nil {
		t.Fatalf("unmarshal synology_meta: %v", err)
	}
	if meta.Model != "DS920+" {
		t.Errorf("model: got %q, want \"DS920+\"", meta.Model)
	}
	if meta.CPUPercent != 25.0 {
		t.Errorf("cpu_percent: got %v, want 25.0", meta.CPUPercent)
	}
	if len(meta.Volumes) != 1 {
		t.Errorf("volumes: got %d, want 1", len(meta.Volumes))
	}
}
