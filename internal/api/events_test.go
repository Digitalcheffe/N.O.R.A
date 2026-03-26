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
	api.NewAppsHandler(appRepo).Routes(r)
	api.NewEventsHandler(eventRepo).Routes(r)
	return r, db
}

// insertEvent inserts an event directly into the DB and returns it.
func insertEvent(t *testing.T, db *sqlx.DB, appID, severity, displayText string, at time.Time) models.Event {
	t.Helper()
	ev := models.Event{
		ID:          uuid.New().String(),
		AppID:       appID,
		ReceivedAt:  at.UTC().Truncate(time.Second),
		Severity:    severity,
		DisplayText: displayText,
		RawPayload:  `{"source":"test"}`,
		Fields:      `{"event_type":"test"}`,
	}
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO events (id, app_id, received_at, severity, display_text, raw_payload, fields)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ev.ID, ev.AppID, ev.ReceivedAt, ev.Severity, ev.DisplayText, ev.RawPayload, ev.Fields)
	if err != nil {
		t.Fatalf("insertEvent: %v", err)
	}
	return ev
}

// eventsListResponse mirrors the listEventsResponse shape for decoding in tests.
type eventsListResponse struct {
	Data []struct {
		ID          string                 `json:"id"`
		AppID       string                 `json:"app_id"`
		AppName     string                 `json:"app_name"`
		Severity    string                 `json:"severity"`
		DisplayText string                 `json:"display_text"`
		Fields      map[string]interface{} `json:"fields"`
		RawPayload  interface{}            `json:"raw_payload"`
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

func TestListEvents_AppNamePopulated(t *testing.T) {
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
	if resp.Data[0].AppName != "Radarr" {
		t.Errorf("expected app_name=Radarr got %q", resp.Data[0].AppName)
	}
}

func TestListEvents_RawPayloadExcluded(t *testing.T) {
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
	if resp.Data[0].RawPayload != nil {
		t.Errorf("expected raw_payload to be absent from list response, got %v", resp.Data[0].RawPayload)
	}
}

func TestListEvents_FieldsIsObject(t *testing.T) {
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
	// fields must be a JSON object, not a string
	if resp.Data[0].Fields == nil {
		t.Error("expected fields to be a JSON object, got nil")
	}
}

// --- Filter: app_id ---

func TestListEvents_FilterByAppID(t *testing.T) {
	router, db := newEventsTestSetup(t)
	app1 := createApp(t, router, "Sonarr")
	app2 := createApp(t, router, "Radarr")
	now := time.Now()
	insertEvent(t, db, app1.ID, "info", "sonarr event", now)
	insertEvent(t, db, app2.ID, "info", "radarr event", now)

	req := httptest.NewRequest(http.MethodGet, "/events?app_id="+app1.ID, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	resp := decodeEventsResp(t, rr)
	if resp.Total != 1 {
		t.Errorf("expected total=1 got %d", resp.Total)
	}
	if resp.Data[0].AppID != app1.ID {
		t.Errorf("expected app_id=%s got %s", app1.ID, resp.Data[0].AppID)
	}
}

// --- Filter: severity ---

func TestListEvents_FilterBySingleSeverity(t *testing.T) {
	router, db := newEventsTestSetup(t)
	app := createApp(t, router, "Sonarr")
	now := time.Now()
	insertEvent(t, db, app.ID, "info", "info event", now)
	insertEvent(t, db, app.ID, "warn", "warn event", now.Add(time.Second))
	insertEvent(t, db, app.ID, "error", "error event", now.Add(2*time.Second))

	req := httptest.NewRequest(http.MethodGet, "/events?severity=warn", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	resp := decodeEventsResp(t, rr)
	if resp.Total != 1 {
		t.Errorf("expected total=1 got %d", resp.Total)
	}
	if resp.Data[0].Severity != "warn" {
		t.Errorf("expected severity=warn got %s", resp.Data[0].Severity)
	}
}

func TestListEvents_FilterByMultipleSeverities(t *testing.T) {
	router, db := newEventsTestSetup(t)
	app := createApp(t, router, "Sonarr")
	now := time.Now()
	insertEvent(t, db, app.ID, "info", "info", now)
	insertEvent(t, db, app.ID, "warn", "warn", now.Add(time.Second))
	insertEvent(t, db, app.ID, "error", "error", now.Add(2*time.Second))
	insertEvent(t, db, app.ID, "critical", "critical", now.Add(3*time.Second))

	req := httptest.NewRequest(http.MethodGet, "/events?severity=warn,error,critical", nil)
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
	if resp.Data[0].DisplayText != "new" {
		t.Errorf("expected 'new' event got %q", resp.Data[0].DisplayText)
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
	if resp.Data[0].DisplayText != "old" {
		t.Errorf("expected 'old' event got %q", resp.Data[0].DisplayText)
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
		ID         string                 `json:"id"`
		Severity   string                 `json:"severity"`
		RawPayload map[string]interface{} `json:"raw_payload"`
		Fields     map[string]interface{} `json:"fields"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.ID != ev.ID {
		t.Errorf("expected id=%s got %s", ev.ID, detail.ID)
	}
	if detail.Severity != "error" {
		t.Errorf("expected severity=error got %s", detail.Severity)
	}
	// raw_payload must be present and be an object in the detail response
	if detail.RawPayload == nil {
		t.Error("expected raw_payload to be present in detail response")
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
	if resp.Data[0].AppID != app1.ID {
		t.Errorf("expected app_id=%s got %s", app1.ID, resp.Data[0].AppID)
	}
}

func TestListByApp_WithSeverityFilter(t *testing.T) {
	router, db := newEventsTestSetup(t)
	app := createApp(t, router, "Sonarr")
	now := time.Now()
	insertEvent(t, db, app.ID, "info", "info", now)
	insertEvent(t, db, app.ID, "error", "error", now.Add(time.Second))

	req := httptest.NewRequest(http.MethodGet, "/apps/"+app.ID+"/events?severity=error", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	resp := decodeEventsResp(t, rr)
	if resp.Total != 1 {
		t.Errorf("expected total=1 got %d", resp.Total)
	}
	if resp.Data[0].Severity != "error" {
		t.Errorf("expected severity=error got %s", resp.Data[0].Severity)
	}
}
