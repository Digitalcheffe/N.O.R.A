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
	cfg := &config.Config{DBPath: ":memory:", DevMode: true}
	db, err := repo.Open(cfg, migrations.Files)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	appRepo := repo.NewAppRepo(db)
	eventRepo := repo.NewEventRepo(db)
	return repo.NewStore(appRepo, eventRepo, repo.NewCheckRepo(db), repo.NewRollupRepo(db), nil, nil, nil, nil, nil, nil, nil, nil, nil)
}

func seedApp(t *testing.T, store *repo.Store, token string, rateLimit int) models.App {
	t.Helper()
	app := &models.App{
		ID:        uuid.NewString(),
		Name:      "test-app",
		Token:     token,
		ProfileID: "",
		RateLimit: rateLimit,
		Config:    json.RawMessage("{}"),
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
	if ev.AppID != app.ID {
		t.Errorf("app_id: want %s got %s", app.ID, ev.AppID)
	}
	if ev.Severity != "info" {
		t.Errorf("severity: want info got %s", ev.Severity)
	}
	if ev.DisplayText != "Event received" {
		t.Errorf("display_text: want 'Event received' got %s", ev.DisplayText)
	}
	if ev.RawPayload != `{"event":"test"}` {
		t.Errorf("raw_payload mismatch: %s", ev.RawPayload)
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
		Config:    json.RawMessage("{}"),
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
	if ev.Severity != "info" {
		t.Errorf("severity: want info got %s", ev.Severity)
	}
	if ev.DisplayText != "Breaking Bad — Grabbed" {
		t.Errorf("display_text: want 'Breaking Bad — Grabbed' got %s", ev.DisplayText)
	}
}

// stubLoader returns a fixed app template for any templateID.
type stubLoader struct {
	template *apptemplate.AppTemplate
}

func (s *stubLoader) Get(_ string) (*apptemplate.AppTemplate, error) {
	return s.template, nil
}
