package ingest_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/digitalcheffe/nora/internal/apptemplate"
	"github.com/digitalcheffe/nora/internal/config"
	"github.com/digitalcheffe/nora/internal/ingest"
	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
	"github.com/digitalcheffe/nora/migrations"
	"github.com/google/uuid"
)

func newTestStore(t *testing.T) *repo.Store {
	t.Helper()
	cfg := &config.Config{DBPath: ":memory:"}
	db, err := repo.Open(cfg, migrations.Files)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	appRepo := repo.NewAppRepo(db)
	eventRepo := repo.NewEventRepo(db)
	return repo.NewStore(appRepo, eventRepo, repo.NewCheckRepo(db), repo.NewRollupRepo(db), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
}

func seedApp(t *testing.T, store *repo.Store, token string, rateLimit int) models.App {
	t.Helper()
	app := &models.App{
		ID:        uuid.NewString(),
		Name:      "test-app",
		Token:     token,
		ProfileID: "",
		RateLimit: rateLimit,
		Config:    models.ConfigJSON("{}"),
	}
	if err := store.Apps.Create(context.Background(), app); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	return *app
}

func TestProcess_HappyPath(t *testing.T) {
	store := newTestStore(t)
	token := "test-token-abc"
	app := seedApp(t, store, token, 100)

	limiter := ingest.NewRateLimiter()
	result, err := ingest.Process(context.Background(), store, &apptemplate.NoopLoader{}, limiter, token, []byte(`{"event":"test"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.EventID == "" {
		t.Fatal("expected non-empty EventID")
	}

	// Verify the event is persisted.
	ev, err := store.Events.Get(context.Background(), result.EventID)
	if err != nil {
		t.Fatalf("get persisted event: %v", err)
	}
	if ev.SourceID != app.ID {
		t.Errorf("source_id: want %s got %s", app.ID, ev.SourceID)
	}
	if ev.SourceType != "app" {
		t.Errorf("source_type: want app got %s", ev.SourceType)
	}
	if ev.Level != "info" {
		t.Errorf("level: want info got %s", ev.Level)
	}
	if ev.Title != "Event received" {
		t.Errorf("title: want 'Event received' got %s", ev.Title)
	}
}

func TestProcess_InvalidToken(t *testing.T) {
	store := newTestStore(t)
	limiter := ingest.NewRateLimiter()

	_, err := ingest.Process(context.Background(), store, &apptemplate.NoopLoader{}, limiter, "no-such-token", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
	var invalidToken ingest.ErrInvalidToken
	if !errors.As(err, &invalidToken) {
		t.Errorf("expected ErrInvalidToken, got %T: %v", err, err)
	}
}

func TestProcess_RateLimit(t *testing.T) {
	store := newTestStore(t)
	token := "rate-test-token"
	seedApp(t, store, token, 2) // limit of 2 events/min

	limiter := ingest.NewRateLimiter()
	payload := []byte(`{"x":1}`)

	// First two should succeed.
	for i := 0; i < 2; i++ {
		_, err := ingest.Process(context.Background(), store, &apptemplate.NoopLoader{}, limiter, token, payload)
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i+1, err)
		}
	}

	// Third should be rate-limited.
	_, err := ingest.Process(context.Background(), store, &apptemplate.NoopLoader{}, limiter, token, payload)
	if err == nil {
		t.Fatal("expected rate limit error on third call")
	}
	var rateLimited ingest.ErrRateLimited
	if !errors.As(err, &rateLimited) {
		t.Errorf("expected ErrRateLimited, got %T: %v", err, err)
	}
}

func TestProcess_ProfileFieldExtraction(t *testing.T) {
	store := newTestStore(t)
	token := "profile-token"
	app := &models.App{
		ID:        uuid.NewString(),
		Name:      "sonarr",
		Token:     token,
		ProfileID: "sonarr",
		RateLimit: 100,
		Config:    models.ConfigJSON("{}"),
	}
	if err := store.Apps.Create(context.Background(), app); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	// App template with field mappings, severity mapping, and display template.
	tmpl := &apptemplate.AppTemplate{
		Webhook: apptemplate.Webhook{
			FieldMappings: map[string]string{
				"eventType": "$.eventType",
				"series":    "$.series.title",
			},
			SeverityField: "eventType",
			SeverityMapping: map[string]string{
				"Test":    "info",
				"Health":  "warn",
				"Grabbed": "info",
			},
			DisplayTemplate: "{series} — {eventType}",
		},
	}

	loader := &stubLoader{template: tmpl}
	limiter := ingest.NewRateLimiter()
	payload := []byte(`{"eventType":"Grabbed","series":{"title":"Breaking Bad"}}`)

	result, err := ingest.Process(context.Background(), store, loader, limiter, token, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ev, err := store.Events.Get(context.Background(), result.EventID)
	if err != nil {
		t.Fatalf("get event: %v", err)
	}
	if ev.Level != "info" {
		t.Errorf("level: want info got %s", ev.Level)
	}
	if ev.Title != "Breaking Bad — Grabbed" {
		t.Errorf("title: want 'Breaking Bad — Grabbed' got %s", ev.Title)
	}
}

// TestProcess_UnmatchedEventType verifies that events whose payload doesn't
// satisfy the display template (e.g. a Sonarr "Test" ping with no series data)
// are still persisted and get a clean display_text instead of raw {placeholders}.
func TestProcess_UnmatchedEventType(t *testing.T) {
	store := newTestStore(t)
	token := "unmatched-token"
	app := &models.App{
		ID:        uuid.NewString(),
		Name:      "sonarr",
		Token:     token,
		ProfileID: "sonarr",
		RateLimit: 100,
		Config:    models.ConfigJSON("{}"),
	}
	if err := store.Apps.Create(context.Background(), app); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	// Template designed for Download events — won't resolve for a Test ping.
	tmpl := &apptemplate.AppTemplate{
		Webhook: apptemplate.Webhook{
			FieldMappings: map[string]string{
				"event_type":   "$.eventType",
				"series_title": "$.series.title",
			},
			SeverityField:   "event_type",
			SeverityMapping: map[string]string{"Download": "info"},
			DisplayTemplate: "{event_type} — {series_title}",
		},
	}

	loader := &stubLoader{template: tmpl}
	limiter := ingest.NewRateLimiter()
	// Sonarr Test ping: has eventType but no series data.
	payload := []byte(`{"eventType":"Test","instanceName":"Sonarr"}`)

	result, err := ingest.Process(context.Background(), store, loader, limiter, token, payload)
	if err != nil {
		t.Fatalf("unexpected error — event must always be persisted: %v", err)
	}

	ev, err := store.Events.Get(context.Background(), result.EventID)
	if err != nil {
		t.Fatalf("get event: %v", err)
	}
	// Must fall back to event_type, not leave raw {placeholders} in title.
	if ev.Title != "Test" {
		t.Errorf("title: want 'Test' got %q", ev.Title)
	}
	if ev.Level != "info" {
		t.Errorf("level: want info got %s", ev.Level)
	}
}

// TestProcess_CompoundSeverity verifies that Health events resolve severity using
// the compound key (eventType + level) when severity_compound_field is configured.
func TestProcess_CompoundSeverity(t *testing.T) {
	store := newTestStore(t)
	token := "compound-sev-token"
	app := &models.App{
		ID:        uuid.NewString(),
		Name:      "sonarr",
		Token:     token,
		ProfileID: "sonarr",
		RateLimit: 100,
		Config:    models.ConfigJSON("{}"),
	}
	if err := store.Apps.Create(context.Background(), app); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	tmpl := &apptemplate.AppTemplate{
		Webhook: apptemplate.Webhook{
			FieldMappings: map[string]string{
				"event_type": "$.eventType",
				"level":      "$.level",
				"message":    "$.message",
			},
			SeverityField:        "event_type",
			SeverityCompoundField: "level",
			SeverityMapping: map[string]string{
				"Health":         "warn",
				"Health:error":   "error",
				"Health:warning": "warn",
				"HealthRestored": "info",
				"Download":       "info",
			},
			DisplayTemplate: "{event_type}",
		},
	}

	cases := []struct {
		payload     string
		wantSev     string
		description string
	}{
		{`{"eventType":"Health","level":"error","message":"Indexer unavailable"}`, "error", "Health:error → error"},
		{`{"eventType":"Health","level":"warning","message":"Indexer degraded"}`, "warn", "Health:warning → warn"},
		{`{"eventType":"Health","message":"Indexer issue"}`, "warn", "Health (no level) → warn"},
		{`{"eventType":"HealthRestored","message":"Indexer back"}`, "info", "HealthRestored → info"},
		{`{"eventType":"Download"}`, "info", "Download → info"},
	}

	loader := &stubLoader{template: tmpl}
	limiter := ingest.NewRateLimiter()

	for _, c := range cases {
		result, err := ingest.Process(context.Background(), store, loader, limiter, token, []byte(c.payload))
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.description, err)
		}
		ev, err := store.Events.Get(context.Background(), result.EventID)
		if err != nil {
			t.Fatalf("get event: %v", err)
		}
		if ev.Level != c.wantSev {
			t.Errorf("%s: level = %q, want %q", c.description, ev.Level, c.wantSev)
		}
	}
}

// TestProcess_ArrayFieldExtraction verifies that field paths using array indexing
// (e.g. $.episodes[0].seasonNumber) are resolved correctly during ingest.
func TestProcess_ArrayFieldExtraction(t *testing.T) {
	store := newTestStore(t)
	token := "array-token"
	app := &models.App{
		ID:        uuid.NewString(),
		Name:      "sonarr",
		Token:     token,
		ProfileID: "sonarr",
		RateLimit: 100,
		Config:    models.ConfigJSON("{}"),
	}
	if err := store.Apps.Create(context.Background(), app); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	tmpl := &apptemplate.AppTemplate{
		Webhook: apptemplate.Webhook{
			FieldMappings: map[string]string{
				"event_type":   "$.eventType",
				"series_title": "$.series.title",
				"season":       "$.episodes[0].seasonNumber",
				"episode":      "$.episodes[0].episodeNumber",
			},
			SeverityField:   "event_type",
			SeverityMapping: map[string]string{"Download": "info"},
			DisplayTemplates: map[string]string{
				"Download": "Download — {series_title} S{season}E{episode}",
			},
			DisplayTemplate: "{event_type} — {series_title}",
		},
	}

	loader := &stubLoader{template: tmpl}
	limiter := ingest.NewRateLimiter()
	payload := []byte(`{
		"eventType": "Download",
		"series": {"title": "The Expanse"},
		"episodes": [{"seasonNumber": 2, "episodeNumber": 7}]
	}`)

	result, err := ingest.Process(context.Background(), store, loader, limiter, token, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ev, err := store.Events.Get(context.Background(), result.EventID)
	if err != nil {
		t.Fatalf("get event: %v", err)
	}
	if ev.Title != "Download — The Expanse S2E7" {
		t.Errorf("title: want 'Download — The Expanse S2E7' got %q", ev.Title)
	}
}

// TestProcess_PerEventTypeTemplate verifies that Health events use the per-eventType template.
func TestProcess_PerEventTypeTemplate(t *testing.T) {
	store := newTestStore(t)
	token := "health-token"
	app := &models.App{
		ID:        uuid.NewString(),
		Name:      "sonarr",
		Token:     token,
		ProfileID: "sonarr",
		RateLimit: 100,
		Config:    models.ConfigJSON("{}"),
	}
	if err := store.Apps.Create(context.Background(), app); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	tmpl := &apptemplate.AppTemplate{
		Webhook: apptemplate.Webhook{
			FieldMappings: map[string]string{
				"event_type": "$.eventType",
				"message":    "$.message",
			},
			SeverityField:   "event_type",
			SeverityMapping: map[string]string{"Health": "warn"},
			DisplayTemplates: map[string]string{
				"Health":         "Health Issue — {message}",
				"HealthRestored": "Health Restored — {message}",
			},
			DisplayTemplate: "{event_type}",
		},
	}

	cases := []struct {
		eventType   string
		message     string
		wantDisplay string
		wantSev     string
	}{
		{"Health", "Indexer search failed", "Health Issue — Indexer search failed", "warn"},
		{"HealthRestored", "Indexer back online", "Health Restored — Indexer back online", "info"},
		{"Test", "", "Test", "info"},
	}

	loader := &stubLoader{template: tmpl}
	limiter := ingest.NewRateLimiter()

	for _, c := range cases {
		payload, _ := json.Marshal(map[string]string{
			"eventType": c.eventType,
			"message":   c.message,
		})
		result, err := ingest.Process(context.Background(), store, loader, limiter, token, payload)
		if err != nil {
			t.Fatalf("eventType=%q: unexpected error: %v", c.eventType, err)
		}
		ev, err := store.Events.Get(context.Background(), result.EventID)
		if err != nil {
			t.Fatalf("get event: %v", err)
		}
		if ev.Title != c.wantDisplay {
			t.Errorf("eventType=%q title: want %q got %q", c.eventType, c.wantDisplay, ev.Title)
		}
		if ev.Level != c.wantSev {
			t.Errorf("eventType=%q level: want %q got %q", c.eventType, c.wantSev, ev.Level)
		}
	}
}

// stubLoader returns a fixed app template for any templateID.
type stubLoader struct {
	template *apptemplate.AppTemplate
}

func (s *stubLoader) Get(_ string) (*apptemplate.AppTemplate, error) {
	return s.template, nil
}
