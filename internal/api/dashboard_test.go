package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/api"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/profile"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// newDashboardTestSetup creates an in-memory DB + router with the dashboard handler.
func newDashboardTestSetup(t *testing.T, profiler profile.Loader) (http.Handler, *sqlx.DB) {
	t.Helper()
	db := newTestDB(t)
	appRepo := repo.NewAppRepo(db)
	eventRepo := repo.NewEventRepo(db)
	checkRepo := repo.NewCheckRepo(db)
	rollupRepo := repo.NewRollupRepo(db)
	r := chi.NewRouter()
	api.NewDashboardHandler(appRepo, eventRepo, checkRepo, rollupRepo, profiler).Routes(r)
	return r, db
}

// insertApp inserts an app directly into the DB and returns it.
func insertApp(t *testing.T, db *sqlx.DB, name, profileID string) models.App {
	t.Helper()
	app := models.App{
		ID:        uuid.New().String(),
		Name:      name,
		Token:     uuid.New().String(),
		ProfileID: profileID,
		Config:    "{}",
		RateLimit: 100,
	}
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO apps (id, name, token, profile_id, config, rate_limit) VALUES (?,?,?,?,?,?)`,
		app.ID, app.Name, app.Token, app.ProfileID, app.Config, app.RateLimit)
	if err != nil {
		t.Fatalf("insertApp: %v", err)
	}
	return app
}

// insertEventWithFields inserts an event with the given fields JSON.
func insertEventWithFields(t *testing.T, db *sqlx.DB, appID, severity, displayText, fieldsJSON string, at time.Time) models.Event {
	t.Helper()
	ev := models.Event{
		ID:          uuid.New().String(),
		AppID:       appID,
		ReceivedAt:  at.UTC().Truncate(time.Second),
		Severity:    severity,
		DisplayText: displayText,
		RawPayload:  `{}`,
		Fields:      fieldsJSON,
	}
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO events (id, app_id, received_at, severity, display_text, raw_payload, fields)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ev.ID, ev.AppID, ev.ReceivedAt, ev.Severity, ev.DisplayText, ev.RawPayload, ev.Fields)
	if err != nil {
		t.Fatalf("insertEventWithFields: %v", err)
	}
	return ev
}

// makeTestProfiler returns a Loader that serves a minimal profile with digest categories.
func makeTestProfiler(profileID string, categories []profile.DigestCategory) profile.Loader {
	return &staticTestLoader{
		profileID: profileID,
		profile: &profile.Profile{
			Digest: profile.Digest{Categories: categories},
		},
	}
}

type staticTestLoader struct {
	profileID string
	profile   *profile.Profile
}

func (l *staticTestLoader) Get(id string) (*profile.Profile, error) {
	if id == l.profileID {
		return l.profile, nil
	}
	return nil, nil
}

// --- zero app scenario ---

func TestDashboardSummary_ZeroApps(t *testing.T) {
	handler, _ := newDashboardTestSetup(t, &profile.NoopLoader{})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/summary", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// summary_bar must be an empty array
	var summaryBar []interface{}
	if err := json.Unmarshal(resp["summary_bar"], &summaryBar); err != nil {
		t.Fatalf("unmarshal summary_bar: %v", err)
	}
	if len(summaryBar) != 0 {
		t.Errorf("expected empty summary_bar, got %d items", len(summaryBar))
	}

	// apps must be an empty array
	var apps []interface{}
	if err := json.Unmarshal(resp["apps"], &apps); err != nil {
		t.Fatalf("unmarshal apps: %v", err)
	}
	if len(apps) != 0 {
		t.Errorf("expected empty apps, got %d items", len(apps))
	}

	// status must be "normal"
	var status string
	if err := json.Unmarshal(resp["status"], &status); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if status != "normal" {
		t.Errorf("expected status=normal, got %q", status)
	}
}

// --- single app scenario ---

func TestDashboardSummary_SingleApp(t *testing.T) {
	categories := []profile.DigestCategory{
		{Label: "Downloads", MatchField: "event_type", MatchValue: "Download"},
		{Label: "Errors", MatchSeverity: "error"},
	}
	profiler := makeTestProfiler("sonarr", categories)
	handler, db := newDashboardTestSetup(t, profiler)

	app := insertApp(t, db, "Sonarr", "sonarr")

	now := time.Now().UTC()

	// Insert events: 3 downloads, 1 error, 1 info (should not appear as Errors category)
	for i := 0; i < 3; i++ {
		insertEventWithFields(t, db, app.ID, "info", "Download — Show",
			`{"event_type":"Download"}`, now.Add(-time.Duration(i)*time.Hour))
	}
	insertEventWithFields(t, db, app.ID, "error", "Health issue",
		`{"event_type":"HealthIssue"}`, now.Add(-2*time.Hour))
	insertEventWithFields(t, db, app.ID, "info", "Misc event",
		`{"event_type":"Other"}`, now.Add(-3*time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/dashboard/summary?period=week", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Status     string `json:"status"`
		Period     string `json:"period"`
		SummaryBar []struct {
			Label     string `json:"label"`
			Count     int    `json:"count"`
			Sparkline [7]int `json:"sparkline"`
		} `json:"summary_bar"`
		Apps []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Stats     []struct {
				Label string `json:"label"`
				Value string `json:"value"`
			} `json:"stats"`
			Sparkline [7]int `json:"sparkline"`
		} `json:"apps"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// period echoed back
	if resp.Period != "week" {
		t.Errorf("expected period=week, got %q", resp.Period)
	}

	// summary_bar should have 2 entries: Downloads (3) and Errors (1)
	if len(resp.SummaryBar) != 2 {
		t.Fatalf("expected 2 summary_bar items, got %d: %+v", len(resp.SummaryBar), resp.SummaryBar)
	}
	for _, item := range resp.SummaryBar {
		switch item.Label {
		case "Downloads":
			if item.Count != 3 {
				t.Errorf("Downloads: expected count=3, got %d", item.Count)
			}
		case "Errors":
			if item.Count != 1 {
				t.Errorf("Errors: expected count=1, got %d", item.Count)
			}
		default:
			t.Errorf("unexpected summary_bar label: %q", item.Label)
		}
		// sparkline always 7 elements — verify it's there (length guaranteed by [7]int)
		_ = item.Sparkline
	}

	// apps should have 1 entry
	if len(resp.Apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(resp.Apps))
	}
	if resp.Apps[0].Name != "Sonarr" {
		t.Errorf("expected app name Sonarr, got %q", resp.Apps[0].Name)
	}

	// app stats should reflect the categories with data
	if len(resp.Apps[0].Stats) == 0 {
		t.Error("expected at least one app stat")
	}
}

// --- multi-app scenario ---

func TestDashboardSummary_MultiApp(t *testing.T) {
	// Two apps sharing the same "Downloads" category label
	sonarrCats := []profile.DigestCategory{
		{Label: "Downloads", MatchField: "event_type", MatchValue: "Download"},
	}
	radarrCats := []profile.DigestCategory{
		{Label: "Downloads", MatchField: "event_type", MatchValue: "Download"},
		{Label: "Errors", MatchSeverity: "error"},
	}

	// Multi-profile loader
	profiler := &multiTestLoader{
		profiles: map[string]*profile.Profile{
			"sonarr": {Digest: profile.Digest{Categories: sonarrCats}},
			"radarr": {Digest: profile.Digest{Categories: radarrCats}},
		},
	}
	handler, db := newDashboardTestSetup(t, profiler)

	sonarr := insertApp(t, db, "Sonarr", "sonarr")
	radarr := insertApp(t, db, "Radarr", "radarr")

	now := time.Now().UTC()

	// Sonarr: 5 downloads
	for i := 0; i < 5; i++ {
		insertEventWithFields(t, db, sonarr.ID, "info", "Download",
			`{"event_type":"Download"}`, now.Add(-time.Duration(i)*time.Hour))
	}
	// Radarr: 3 downloads, 2 errors
	for i := 0; i < 3; i++ {
		insertEventWithFields(t, db, radarr.ID, "info", "Download",
			`{"event_type":"Download"}`, now.Add(-time.Duration(i)*time.Hour))
	}
	for i := 0; i < 2; i++ {
		insertEventWithFields(t, db, radarr.ID, "error", "Health issue",
			`{"event_type":"HealthIssue"}`, now.Add(-time.Duration(i+3)*time.Hour))
	}

	// An app with no events this period should not create an empty category
	_ = insertApp(t, db, "Duplicati", "duplicati")
	// (no events inserted for duplicati)

	req := httptest.NewRequest(http.MethodGet, "/dashboard/summary?period=week", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		SummaryBar []struct {
			Label string `json:"label"`
			Count int    `json:"count"`
			Sub   string `json:"sub"`
		} `json:"summary_bar"`
		Apps []struct {
			ID        string `json:"id"`
			Sparkline [7]int `json:"sparkline"`
		} `json:"apps"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Downloads from both apps should be combined: 5+3=8
	var downloadsItem *struct {
		Label string `json:"label"`
		Count int    `json:"count"`
		Sub   string `json:"sub"`
	}
	for i := range resp.SummaryBar {
		if resp.SummaryBar[i].Label == "Downloads" {
			downloadsItem = &resp.SummaryBar[i]
		}
	}
	if downloadsItem == nil {
		t.Fatal("expected Downloads in summary_bar")
	}
	if downloadsItem.Count != 8 {
		t.Errorf("Downloads: expected count=8, got %d", downloadsItem.Count)
	}

	// Errors (2) should be present
	var errorsPresent bool
	for _, item := range resp.SummaryBar {
		if item.Label == "Errors" && item.Count == 2 {
			errorsPresent = true
		}
	}
	if !errorsPresent {
		t.Errorf("expected Errors with count=2 in summary_bar, got: %+v", resp.SummaryBar)
	}

	// All 3 apps should appear in apps
	if len(resp.Apps) != 3 {
		t.Errorf("expected 3 apps, got %d", len(resp.Apps))
	}

	// Sparklines always have 7 elements (guaranteed by [7]int type)
	for _, a := range resp.Apps {
		_ = a.Sparkline
	}
}

// multiTestLoader serves multiple profiles.
type multiTestLoader struct {
	profiles map[string]*profile.Profile
}

func (l *multiTestLoader) Get(id string) (*profile.Profile, error) {
	if p, ok := l.profiles[id]; ok {
		return p, nil
	}
	return nil, nil
}

// --- period parameter tests ---

func TestDashboardSummary_PeriodDefault(t *testing.T) {
	handler, _ := newDashboardTestSetup(t, &profile.NoopLoader{})

	// no period param — should default to "week"
	req := httptest.NewRequest(http.MethodGet, "/dashboard/summary", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Period string `json:"period"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Period != "week" {
		t.Errorf("expected default period=week, got %q", resp.Period)
	}
}

func TestDashboardSummary_PeriodDay(t *testing.T) {
	handler, _ := newDashboardTestSetup(t, &profile.NoopLoader{})
	req := httptest.NewRequest(http.MethodGet, "/dashboard/summary?period=day", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp struct {
		Period string `json:"period"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Period != "day" {
		t.Errorf("expected period=day, got %q", resp.Period)
	}
}

func TestDashboardSummary_PeriodMonth(t *testing.T) {
	handler, _ := newDashboardTestSetup(t, &profile.NoopLoader{})
	req := httptest.NewRequest(http.MethodGet, "/dashboard/summary?period=month", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp struct {
		Period string `json:"period"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Period != "month" {
		t.Errorf("expected period=month, got %q", resp.Period)
	}
}

// --- digest endpoint ---

func TestDashboardDigest_Empty(t *testing.T) {
	handler, _ := newDashboardTestSetup(t, &profile.NoopLoader{})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/digest/2026-03", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Period     string        `json:"period"`
		Categories []interface{} `json:"categories"`
		UptimePct  float64       `json:"uptime_pct"`
		ErrorCount int           `json:"error_count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Period != "2026-03" {
		t.Errorf("expected period=2026-03, got %q", resp.Period)
	}
	if len(resp.Categories) != 0 {
		t.Errorf("expected empty categories, got %d", len(resp.Categories))
	}
}

func TestDashboardDigest_WithRollups(t *testing.T) {
	handler, db := newDashboardTestSetup(t, &profile.NoopLoader{})

	app := insertApp(t, db, "Sonarr", "sonarr")

	// Insert rollup rows
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO rollups (app_id, year, month, event_type, severity, count) VALUES (?,?,?,?,?,?)`,
		app.ID, 2026, 3, "Download", "info", 89)
	if err != nil {
		t.Fatalf("insert rollup: %v", err)
	}
	_, err = db.ExecContext(context.Background(),
		`INSERT INTO rollups (app_id, year, month, event_type, severity, count) VALUES (?,?,?,?,?,?)`,
		app.ID, 2026, 3, "HealthIssue", "error", 2)
	if err != nil {
		t.Fatalf("insert rollup: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/dashboard/digest/2026-03", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Period     string `json:"period"`
		ErrorCount int    `json:"error_count"`
		Categories []struct {
			Label string `json:"label"`
			Count int    `json:"count"`
		} `json:"categories"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.ErrorCount != 2 {
		t.Errorf("expected error_count=2, got %d", resp.ErrorCount)
	}
	if len(resp.Categories) != 2 {
		t.Errorf("expected 2 categories, got %d", len(resp.Categories))
	}
}

func TestDashboardDigest_InvalidPeriod(t *testing.T) {
	handler, _ := newDashboardTestSetup(t, &profile.NoopLoader{})

	req := httptest.NewRequest(http.MethodGet, "/dashboard/digest/notaperiod", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// --- category omission test ---

// TestDashboardSummary_OmitsZeroCategories verifies that categories with count=0
// are not included in summary_bar.
func TestDashboardSummary_OmitsZeroCategories(t *testing.T) {
	categories := []profile.DigestCategory{
		{Label: "Downloads", MatchField: "event_type", MatchValue: "Download"},
		{Label: "Backups", MatchField: "event_type", MatchValue: "Backup"},
	}
	profiler := makeTestProfiler("app1", categories)
	handler, db := newDashboardTestSetup(t, profiler)

	app := insertApp(t, db, "App1", "app1")

	// Only download events — no backup events
	insertEventWithFields(t, db, app.ID, "info", "Download",
		`{"event_type":"Download"}`, time.Now().UTC().Add(-time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/dashboard/summary?period=week", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		SummaryBar []struct {
			Label string `json:"label"`
		} `json:"summary_bar"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if len(resp.SummaryBar) != 1 {
		t.Errorf("expected only 1 summary_bar item (Downloads), got %d: %+v", len(resp.SummaryBar), resp.SummaryBar)
	}
	if len(resp.SummaryBar) > 0 && resp.SummaryBar[0].Label != "Downloads" {
		t.Errorf("expected label=Downloads, got %q", resp.SummaryBar[0].Label)
	}
}
