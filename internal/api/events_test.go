package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/api"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// newEventsTestSetup creates an in-memory DB and a router with both apps and events routes.
func newEventsTestSetup(t *testing.T) (http.Handler, *sqlx.DB) {
	t.Helper()
	db := newTestDB(t)
	appRepo := repo.NewAppRepo(db)
	eventRepo := repo.NewEventRepo(db)
	r := chi.NewRouter()
	api.NewAppsHandler(appRepo, nil, nil, nil).Routes(r)
	api.NewEventsHandler(eventRepo).Routes(r)
	return r, db
}

// insertEvent inserts an app event directly into the DB and returns it.
func insertEvent(t *testing.T, db *sqlx.DB, appID, level, title string, at time.Time) models.Event {
	t.Helper()
	appName := "test-app"
	if appID != "" {
		// Try to look up the actual app name for a realistic source_name.
		_ = db.QueryRowContext(context.Background(), "SELECT name FROM apps WHERE id = ?", appID).Scan(&appName)
	}
	ev := models.Event{
		ID:         uuid.New().String(),
		Level:      level,
		SourceName: appName,
		SourceType: "app",
		SourceID:   appID,
		Title:      title,
		Payload:    `{"source":"test","event_type":"test"}`,
		CreatedAt:  at.UTC().Truncate(time.Second),
	}
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO events (id, level, source_name, source_type, source_id, title, payload, created_at)
		 VALUES (?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?)`,
		ev.ID, ev.Level, ev.SourceName, ev.SourceType, ev.SourceID,
		ev.Title, ev.Payload, ev.CreatedAt)
	if err != nil {
		t.Fatalf("insertEvent: %v", err)
	}
	return ev
}

// eventsListResponse mirrors the listEventsResponse shape for decoding in tests.
type eventsListResponse struct {
	Data []struct {
		ID         string      `json:"id"`
		Level      string      `json:"level"`
		SourceName string      `json:"source_name"`
		SourceType string      `json:"source_type"`
		SourceID   string      `json:"source_id"`
		Title      string      `json:"title"`
		Payload    interface{} `json:"payload"`
	} `json:"data"`
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

func decodeEventsResp(t *testing.T, rr *httptest.ResponseRecorder) eventsListResponse {
	t.Helper()
	var resp eventsListResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode events response: %v", err)
	}
	return resp
}

// --- List ---

func TestListEvents_Empty(t *testing.T) {
	router, _ := newEventsTestSetup(t)

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rr.Code)
	}
	resp := decodeEventsResp(t, rr)
	if resp.Total != 0 {
		t.Errorf("expected total=0 got %d", resp.Total)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected empty data got %d items", len(resp.Data))
	}
	if resp.Limit != 50 {
		t.Errorf("expected default limit=50 got %d", resp.Limit)
	}
}

func TestListEvents_ReturnsAll(t *testing.T) {
	router, db := newEventsTestSetup(t)
	app := createApp(t, router, "Sonarr")
	now := time.Now()
	insertEvent(t, db, app.ID, "info", "event one", now)
	insertEvent(t, db, app.ID, "warn", "event two", now.Add(time.Minute))

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	resp := decodeEventsResp(t, rr)
	if resp.Total != 2 {
		t.Errorf("expected total=2 got %d", resp.Total)
	}
	if len(resp.Data) != 2 {
		t.Errorf("expected 2 items got %d", len(resp.Data))
	}
}

func TestListEvents_SourceNamePopulated(t *testing.T) {
	router, db := newEventsTestSetup(t)
	app := createApp(t, router, "Radarr")
	insertEvent(t, db, app.ID, "info", "download complete", time.Now())

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	resp := decodeEventsResp(t, rr)
	if len(resp.Data) == 0 {
		t.Fatal("expected at least one event")
	}
	if resp.Data[0].SourceName != "Radarr" {
		t.Errorf("expected source_name=Radarr got %q", resp.Data[0].SourceName)
	}
}

func TestListEvents_PayloadExcludedFromList(t *testing.T) {
	router, db := newEventsTestSetup(t)
	app := createApp(t, router, "Sonarr")
	insertEvent(t, db, app.ID, "info", "test", time.Now())

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	resp := decodeEventsResp(t, rr)
	if len(resp.Data) == 0 {
		t.Fatal("expected one event")
	}
	// payload must be absent from list results
	if resp.Data[0].Payload != nil {
		t.Errorf("expected payload to be absent from list response, got %v", resp.Data[0].Payload)
	}
}

// --- Filter: source_id ---

func TestListEvents_FilterBySourceID(t *testing.T) {
	router, db := newEventsTestSetup(t)
	app1 := createApp(t, router, "Sonarr")
	app2 := createApp(t, router, "Radarr")
	now := time.Now()
	insertEvent(t, db, app1.ID, "info", "sonarr event", now)
	insertEvent(t, db, app2.ID, "info", "radarr event", now)

	req := httptest.NewRequest(http.MethodGet, "/events?source_id="+app1.ID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	resp := decodeEventsResp(t, rr)
	if resp.Total != 1 {
		t.Errorf("expected total=1 got %d", resp.Total)
	}
	if resp.Data[0].SourceID != app1.ID {
		t.Errorf("expected source_id=%s got %s", app1.ID, resp.Data[0].SourceID)
	}
}

// --- Filter: level ---

func TestListEvents_FilterBySingleLevel(t *testing.T) {
	router, db := newEventsTestSetup(t)
	app := createApp(t, router, "Sonarr")
	now := time.Now()
	insertEvent(t, db, app.ID, "info", "info event", now)
	insertEvent(t, db, app.ID, "warn", "warn event", now.Add(time.Second))
	insertEvent(t, db, app.ID, "error", "error event", now.Add(2*time.Second))

	req := httptest.NewRequest(http.MethodGet, "/events?level=warn", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	resp := decodeEventsResp(t, rr)
	if resp.Total != 1 {
		t.Errorf("expected total=1 got %d", resp.Total)
	}
	if resp.Data[0].Level != "warn" {
		t.Errorf("expected level=warn got %s", resp.Data[0].Level)
	}
}

func TestListEvents_FilterByMultipleLevels(t *testing.T) {
	router, db := newEventsTestSetup(t)
	app := createApp(t, router, "Sonarr")
	now := time.Now()
	insertEvent(t, db, app.ID, "info", "info", now)
	insertEvent(t, db, app.ID, "warn", "warn", now.Add(time.Second))
	insertEvent(t, db, app.ID, "error", "error", now.Add(2*time.Second))
	insertEvent(t, db, app.ID, "critical", "critical", now.Add(3*time.Second))

	req := httptest.NewRequest(http.MethodGet, "/events?level=warn,error,critical", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	resp := decodeEventsResp(t, rr)
	if resp.Total != 3 {
		t.Errorf("expected total=3 got %d", resp.Total)
	}
}

// --- Filter: since / until ---

func TestListEvents_FilterBySince(t *testing.T) {
	router, db := newEventsTestSetup(t)
	app := createApp(t, router, "Sonarr")
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	insertEvent(t, db, app.ID, "info", "old", base)
	insertEvent(t, db, app.ID, "info", "new", base.Add(2*time.Hour))

	since := base.Add(time.Hour).Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet, "/events?since="+since, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	resp := decodeEventsResp(t, rr)
	if resp.Total != 1 {
		t.Errorf("expected total=1 got %d", resp.Total)
	}
	if resp.Data[0].Title != "new" {
		t.Errorf("expected 'new' event got %q", resp.Data[0].Title)
	}
}

func TestListEvents_FilterByUntil(t *testing.T) {
	router, db := newEventsTestSetup(t)
	app := createApp(t, router, "Sonarr")
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	insertEvent(t, db, app.ID, "info", "old", base)
	insertEvent(t, db, app.ID, "info", "new", base.Add(2*time.Hour))

	until := base.Add(time.Hour).Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet, "/events?until="+until, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	resp := decodeEventsResp(t, rr)
	if resp.Total != 1 {
		t.Errorf("expected total=1 got %d", resp.Total)
	}
	if resp.Data[0].Title != "old" {
		t.Errorf("expected 'old' event got %q", resp.Data[0].Title)
	}
}

// --- Pagination ---

func TestListEvents_LimitOffset(t *testing.T) {
	router, db := newEventsTestSetup(t)
	app := createApp(t, router, "Sonarr")
	now := time.Now()
	for i := 0; i < 5; i++ {
		insertEvent(t, db, app.ID, "info", fmt.Sprintf("event %d", i), now.Add(time.Duration(i)*time.Second))
	}

	req := httptest.NewRequest(http.MethodGet, "/events?limit=2&offset=2", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	resp := decodeEventsResp(t, rr)
	if resp.Total != 5 {
		t.Errorf("expected total=5 got %d", resp.Total)
	}
	if len(resp.Data) != 2 {
		t.Errorf("expected 2 items on page got %d", len(resp.Data))
	}
	if resp.Limit != 2 {
		t.Errorf("expected limit=2 got %d", resp.Limit)
	}
	if resp.Offset != 2 {
		t.Errorf("expected offset=2 got %d", resp.Offset)
	}
}

func TestListEvents_InvalidLimit(t *testing.T) {
	router, _ := newEventsTestSetup(t)
	req := httptest.NewRequest(http.MethodGet, "/events?limit=bad", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

func TestListEvents_InvalidSince(t *testing.T) {
	router, _ := newEventsTestSetup(t)
	req := httptest.NewRequest(http.MethodGet, "/events?since=not-a-date", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 got %d", rr.Code)
	}
}

// --- Get single event ---

func TestGetEvent_HappyPath(t *testing.T) {
	router, db := newEventsTestSetup(t)
	app := createApp(t, router, "Sonarr")
	ev := insertEvent(t, db, app.ID, "error", "disk full", time.Now())

	req := httptest.NewRequest(http.MethodGet, "/events/"+ev.ID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}

	var detail struct {
		ID      string                 `json:"id"`
		Level   string                 `json:"level"`
		Payload map[string]interface{} `json:"payload"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.ID != ev.ID {
		t.Errorf("expected id=%s got %s", ev.ID, detail.ID)
	}
	if detail.Level != "error" {
		t.Errorf("expected level=error got %s", detail.Level)
	}
	// payload must be present and be an object in the detail response
	if detail.Payload == nil {
		t.Error("expected payload to be present in detail response")
	}
}

func TestGetEvent_NotFound(t *testing.T) {
	router, _ := newEventsTestSetup(t)
	req := httptest.NewRequest(http.MethodGet, "/events/does-not-exist", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 got %d", rr.Code)
	}
}

// --- ListByApp ---

func TestListByApp_HappyPath(t *testing.T) {
	router, db := newEventsTestSetup(t)
	app := createApp(t, router, "Sonarr")
	insertEvent(t, db, app.ID, "info", "episode grabbed", time.Now())

	req := httptest.NewRequest(http.MethodGet, "/apps/"+app.ID+"/events", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rr.Code, rr.Body.String())
	}
	resp := decodeEventsResp(t, rr)
	if resp.Total != 1 {
		t.Errorf("expected total=1 got %d", resp.Total)
	}
}

func TestListByApp_ExcludesOtherApps(t *testing.T) {
	router, db := newEventsTestSetup(t)
	app1 := createApp(t, router, "Sonarr")
	app2 := createApp(t, router, "Radarr")
	now := time.Now()
	insertEvent(t, db, app1.ID, "info", "sonarr", now)
	insertEvent(t, db, app2.ID, "info", "radarr", now)

	req := httptest.NewRequest(http.MethodGet, "/apps/"+app1.ID+"/events", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	resp := decodeEventsResp(t, rr)
	if resp.Total != 1 {
		t.Errorf("expected total=1 (only app1 events) got %d", resp.Total)
	}
	if resp.Data[0].SourceID != app1.ID {
		t.Errorf("expected source_id=%s got %s", app1.ID, resp.Data[0].SourceID)
	}
}

func TestListByApp_WithLevelFilter(t *testing.T) {
	router, db := newEventsTestSetup(t)
	app := createApp(t, router, "Sonarr")
	now := time.Now()
	insertEvent(t, db, app.ID, "info", "info", now)
	insertEvent(t, db, app.ID, "error", "error", now.Add(time.Second))

	req := httptest.NewRequest(http.MethodGet, "/apps/"+app.ID+"/events?level=error", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	resp := decodeEventsResp(t, rr)
	if resp.Total != 1 {
		t.Errorf("expected total=1 got %d", resp.Total)
	}
	if resp.Data[0].Level != "error" {
		t.Errorf("expected level=error got %s", resp.Data[0].Level)
	}
}
