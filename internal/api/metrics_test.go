package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/api"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
)

func newMetricsRouter(t *testing.T) http.Handler {
	t.Helper()
	db := newTestDB(t)
	eventRepo := repo.NewEventRepo(db)
	appRepo := repo.NewAppRepo(db)
	metricsRepo := repo.NewMetricsRepo(db)
	h := api.NewMetricsHandler(eventRepo, appRepo, metricsRepo, ":memory:", time.Now())
	r := chi.NewRouter()
	h.Routes(r)
	return r
}

// --- GET /metrics (instance-wide) ---

func TestGetInstanceMetrics_Empty(t *testing.T) {
	router := newMetricsRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		DBSizeBytes   int64 `json:"db_size_bytes"`
		EventsLast24h int   `json:"events_last_24h"`
		UptimeSeconds int64 `json:"uptime_seconds"`
		TopApps       []any `json:"top_apps"`
		AppEvents24h  []any `json:"app_events_24h"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.EventsLast24h != 0 {
		t.Errorf("expected events_last_24h=0 got %d", resp.EventsLast24h)
	}
	if resp.TopApps == nil {
		t.Error("expected top_apps to be a non-nil slice")
	}
	if resp.AppEvents24h == nil {
		t.Error("expected app_events_24h to be a non-nil slice")
	}
}

func TestGetInstanceMetrics_HasFields(t *testing.T) {
	router := newMetricsRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}

	var resp map[string]json.RawMessage
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, field := range []string{"db_size_bytes", "events_last_24h", "uptime_seconds", "top_apps", "app_events_24h"} {
		if _, ok := resp[field]; !ok {
			t.Errorf("missing field %q in response", field)
		}
	}
}

// --- GET /apps/{id}/metrics ---

func TestGetAppMetrics_NotFound(t *testing.T) {
	router := newMetricsRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/apps/does-not-exist/metrics", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}


func TestGetAppMetrics_EmptyTrend(t *testing.T) {
	// Create app via the apps handler, then call metrics on the same DB.
	db := newTestDB(t)
	appRepo := repo.NewAppRepo(db)
	eventRepo := repo.NewEventRepo(db)
	metricsRepo := repo.NewMetricsRepo(db)

	appsHandler := api.NewAppsHandler(appRepo, nil)
	metricsHandler := api.NewMetricsHandler(eventRepo, appRepo, metricsRepo, ":memory:", time.Now())

	r := chi.NewRouter()
	appsHandler.Routes(r)
	metricsHandler.Routes(r)

	app := createApp(t, r, "MetricsApp")

	req := httptest.NewRequest(http.MethodGet, "/apps/"+app.ID+"/metrics", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data  []any `json:"data"`
		Total int   `json:"total"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("expected total=0 got %d", resp.Total)
	}
	if resp.Data == nil {
		t.Error("expected data to be a non-nil slice")
	}
}
