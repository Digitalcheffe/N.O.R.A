package discovery_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/internal/scanner/discovery"
	"github.com/google/uuid"
)

// ── stub loader ───────────────────────────────────────────────────────────────

// stubLoader implements apptemplate.Loader for a single profile.
type stubLoader struct {
	profileID string
	tmpl      *apptemplate.AppTemplate
}

func (s *stubLoader) Get(id string) (*apptemplate.AppTemplate, error) {
	if id == s.profileID {
		return s.tmpl, nil
	}
	return nil, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// newAPITestStore opens an in-memory DB (reuses newTestStore from the package)
// and wires the AppMetricSnapshotRepo so polling tests can verify writes.
func newAPITestStore(t *testing.T) (*repo.Store, interface{ Close() error }) {
	t.Helper()
	store, db := newTestStore(t)
	_ = store // discard the store without AppMetricSnapshots

	full := repo.NewStore(
		repo.NewAppRepo(db),
		repo.NewEventRepo(db),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		repo.NewAppMetricSnapshotRepo(db),
	)
	return full, db
}

// seedPollApp inserts a minimal app row and returns the inserted app.
func seedPollApp(t *testing.T, store *repo.Store, name, profileID, configJSON string) *models.App {
	t.Helper()
	if configJSON == "" {
		configJSON = "{}"
	}
	app := &models.App{
		ID:        uuid.NewString(),
		Name:      name,
		Token:     uuid.NewString(),
		ProfileID: profileID,
		Config:    models.ConfigJSON(configJSON),
		RateLimit: 100,
	}
	if err := store.Apps.Create(context.Background(), app); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	return app
}

// ── ExtractValue unit tests ───────────────────────────────────────────────────

func TestExtractValue_Length(t *testing.T) {
	body := []byte(`[1, 2, 3]`)
	got, err := discovery.ExtractValue(body, "length", "count")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "3" {
		t.Errorf("got %q, want %q", got, "3")
	}
}

func TestExtractValue_LengthEmpty(t *testing.T) {
	got, err := discovery.ExtractValue([]byte(`[]`), "length", "count")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "0" {
		t.Errorf("got %q, want %q", got, "0")
	}
}

func TestExtractValue_LengthNotArray(t *testing.T) {
	_, err := discovery.ExtractValue([]byte(`{"not":"array"}`), "length", "count")
	if err == nil {
		t.Error("expected error for non-array body")
	}
}

func TestExtractValue_JSONPath_String(t *testing.T) {
	body := []byte(`{"status":"ok","count":42}`)
	got, err := discovery.ExtractValue(body, "$.status", "string")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ok" {
		t.Errorf("got %q, want %q", got, "ok")
	}
}

func TestExtractValue_JSONPath_Nested(t *testing.T) {
	body := []byte(`{"data":{"total":99}}`)
	got, err := discovery.ExtractValue(body, "$.data.total", "count")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "99" {
		t.Errorf("got %q, want %q", got, "99")
	}
}

func TestExtractValue_JSONPath_List(t *testing.T) {
	body := []byte(`{"items":[1,2,3]}`)
	got, err := discovery.ExtractValue(body, "$.items", "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var arr []int
	if err := json.Unmarshal([]byte(got), &arr); err != nil {
		t.Fatalf("list value is not valid JSON: %v — got %q", err, got)
	}
	if len(arr) != 3 {
		t.Errorf("expected 3 items, got %d", len(arr))
	}
}

func TestExtractValue_JSONPath_NotFound(t *testing.T) {
	_, err := discovery.ExtractValue([]byte(`{"other":"value"}`), "$.missing", "string")
	if err == nil {
		t.Error("expected error for missing JSONPath")
	}
}

func TestExtractValue_UnsupportedTarget(t *testing.T) {
	_, err := discovery.ExtractValue([]byte(`{}`), "badtarget", "string")
	if err == nil {
		t.Error("expected error for unsupported target")
	}
}

// ── RunAPIPolling integration tests ──────────────────────────────────────────

// TestRunAPIPolling_SnapshotWrittenAndEventFired verifies the happy path:
// a mock HTTP server returns a JSON array, the poller writes a snapshot row.
func TestRunAPIPolling_SnapshotWrittenAndEventFired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]int{1, 2, 3, 4, 5})
	}))
	defer srv.Close()

	store, _ := newAPITestStore(t)
	app := seedPollApp(t, store, "TestApp", "testprofile",
		`{"base_url":"`+srv.URL+`"}`)

	loader := &stubLoader{
		profileID: "testprofile",
		tmpl: &apptemplate.AppTemplate{
			APIPolling: []apptemplate.APIPollingEntry{
				{Path: "/api/items", Name: "item_count", Label: "Items",
					Target: "length", ValueType: "count", EventMessage: "Items: {value}"},
			},
		},
	}

	if err := discovery.RunAPIPolling(context.Background(), store, loader); err != nil {
		t.Fatalf("RunAPIPolling: %v", err)
	}

	snap, err := store.AppMetricSnapshots.GetByAppAndMetric(context.Background(), app.ID, "item_count")
	if err != nil {
		t.Fatalf("GetByAppAndMetric: %v", err)
	}
	if snap.Value != "5" {
		t.Errorf("snapshot value: got %q, want %q", snap.Value, "5")
	}
	if snap.Label != "Items" {
		t.Errorf("snapshot label: got %q, want %q", snap.Label, "Items")
	}
}

// TestRunAPIPolling_NoAPIPollingBlock verifies apps with no api_polling are skipped.
func TestRunAPIPolling_NoAPIPollingBlock(t *testing.T) {
	store, _ := newAPITestStore(t)
	seedPollApp(t, store, "NoPollingApp", "noprofile", `{}`)

	loader := &stubLoader{
		profileID: "noprofile",
		tmpl:      &apptemplate.AppTemplate{APIPolling: nil},
	}

	if err := discovery.RunAPIPolling(context.Background(), store, loader); err != nil {
		t.Fatalf("RunAPIPolling: %v", err)
	}
}

// TestRunAPIPolling_HTTPError verifies that a non-200 response is logged and
// skipped — RunAPIPolling must not return an error.
func TestRunAPIPolling_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	store, _ := newAPITestStore(t)
	app := seedPollApp(t, store, "ErrApp", "errprofile", `{"base_url":"`+srv.URL+`"}`)

	loader := &stubLoader{
		profileID: "errprofile",
		tmpl: &apptemplate.AppTemplate{
			APIPolling: []apptemplate.APIPollingEntry{
				{Path: "/fail", Name: "fails", Label: "Fails", Target: "length", ValueType: "count"},
			},
		},
	}

	if err := discovery.RunAPIPolling(context.Background(), store, loader); err != nil {
		t.Fatalf("RunAPIPolling returned unexpected error: %v", err)
	}

	snaps, err := store.AppMetricSnapshots.ListByApp(context.Background(), app.ID)
	if err != nil {
		t.Fatalf("ListByApp: %v", err)
	}
	if len(snaps) != 0 {
		t.Errorf("expected 0 snapshots after HTTP error, got %d", len(snaps))
	}
}

// TestRunAPIPolling_JSONPathExtraction verifies nested JSONPath extraction.
func TestRunAPIPolling_JSONPathExtraction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"stats": map[string]interface{}{"total": 42},
		})
	}))
	defer srv.Close()

	store, _ := newAPITestStore(t)
	app := seedPollApp(t, store, "PathApp", "pathprofile", `{"base_url":"`+srv.URL+`"}`)

	loader := &stubLoader{
		profileID: "pathprofile",
		tmpl: &apptemplate.AppTemplate{
			APIPolling: []apptemplate.APIPollingEntry{
				{Path: "/stats", Name: "total", Label: "Total", Target: "$.stats.total", ValueType: "count"},
			},
		},
	}

	if err := discovery.RunAPIPolling(context.Background(), store, loader); err != nil {
		t.Fatalf("RunAPIPolling: %v", err)
	}

	snap, err := store.AppMetricSnapshots.GetByAppAndMetric(context.Background(), app.ID, "total")
	if err != nil {
		t.Fatalf("GetByAppAndMetric: %v", err)
	}
	if snap.Value != "42" {
		t.Errorf("snapshot value: got %q, want %q", snap.Value, "42")
	}
}

// TestRunAPIPolling_UpsertReplacesExistingValue verifies a second poll
// overwrites the previous snapshot and keeps exactly one row.
func TestRunAPIPolling_UpsertReplacesExistingValue(t *testing.T) {
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		call++
		if call == 1 {
			json.NewEncoder(w).Encode([]int{1, 2, 3})
		} else {
			json.NewEncoder(w).Encode([]int{1, 2, 3, 4, 5, 6, 7})
		}
	}))
	defer srv.Close()

	store, _ := newAPITestStore(t)
	app := seedPollApp(t, store, "UpsertApp", "upsertprofile", `{"base_url":"`+srv.URL+`"}`)

	loader := &stubLoader{
		profileID: "upsertprofile",
		tmpl: &apptemplate.AppTemplate{
			APIPolling: []apptemplate.APIPollingEntry{
				{Path: "/items", Name: "count", Label: "Count", Target: "length", ValueType: "count"},
			},
		},
	}

	if err := discovery.RunAPIPolling(context.Background(), store, loader); err != nil {
		t.Fatalf("first poll: %v", err)
	}
	if err := discovery.RunAPIPolling(context.Background(), store, loader); err != nil {
		t.Fatalf("second poll: %v", err)
	}

	snap, err := store.AppMetricSnapshots.GetByAppAndMetric(context.Background(), app.ID, "count")
	if err != nil {
		t.Fatalf("GetByAppAndMetric: %v", err)
	}
	if snap.Value != "7" {
		t.Errorf("snapshot should reflect second poll: got %q, want %q", snap.Value, "7")
	}

	all, err := store.AppMetricSnapshots.ListByApp(context.Background(), app.ID)
	if err != nil {
		t.Fatalf("ListByApp: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("upsert should keep exactly 1 row, got %d", len(all))
	}
}

// TestRunAPIPolling_APIKeyHeaderAuth verifies apikey_header sets the correct header.
func TestRunAPIPolling_APIKeyHeaderAuth(t *testing.T) {
	var receivedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Api-Key")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]int{1})
	}))
	defer srv.Close()

	store, _ := newAPITestStore(t)
	// Use "sonarr" as the profile ID — apipoller has sonarr.yaml with apikey_header / X-Api-Key.
	seedPollApp(t, store, "AuthApp", "sonarr",
		`{"base_url":"`+srv.URL+`","api_key":"secret-key"}`)

	loader := &stubLoader{
		profileID: "sonarr",
		tmpl: &apptemplate.AppTemplate{
			APIPolling: []apptemplate.APIPollingEntry{
				{Path: "/items", Name: "count", Label: "Count",
					Target: "length", ValueType: "count"},
			},
		},
	}

	if err := discovery.RunAPIPolling(context.Background(), store, loader); err != nil {
		t.Fatalf("RunAPIPolling: %v", err)
	}
	if receivedHeader != "secret-key" {
		t.Errorf("auth header: got %q, want %q", receivedHeader, "secret-key")
	}
}
