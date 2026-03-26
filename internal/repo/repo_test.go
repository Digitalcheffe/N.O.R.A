package repo

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/migrations"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// openSchemaDB opens an in-memory SQLite database with the full NORA schema applied.
func openSchemaDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open schema db: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	if err := runMigrations(db, migrations.Files); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// --- App repository tests ---

func TestAppRepo_CreateAndGetByID(t *testing.T) {
	db := openSchemaDB(t)
	repo := &sqliteAppRepo{db}
	ctx := context.Background()

	app := &models.App{
		Name:      "Sonarr",
		Token:     "tok-abc123",
		RateLimit: 100,
		Config:    models.RawMessage(`{"url":"http://sonarr:8989"}`),
	}

	if err := repo.Create(ctx, app); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if app.ID == "" {
		t.Fatal("expected ID to be set after Create")
	}
	if app.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt to be set after Create")
	}

	got, err := repo.GetByID(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != app.Name {
		t.Errorf("Name: got %q, want %q", got.Name, app.Name)
	}
	if got.Token != app.Token {
		t.Errorf("Token: got %q, want %q", got.Token, app.Token)
	}
	if got.RateLimit != app.RateLimit {
		t.Errorf("RateLimit: got %d, want %d", got.RateLimit, app.RateLimit)
	}
}

func TestAppRepo_GetByToken(t *testing.T) {
	db := openSchemaDB(t)
	repo := &sqliteAppRepo{db}
	ctx := context.Background()

	app := &models.App{Name: "Radarr", Token: "tok-radarr", RateLimit: 50}
	if err := repo.Create(ctx, app); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.GetByToken(ctx, "tok-radarr")
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if got.ID != app.ID {
		t.Errorf("ID: got %q, want %q", got.ID, app.ID)
	}
}

func TestAppRepo_GetByID_NotFound(t *testing.T) {
	db := openSchemaDB(t)
	repo := &sqliteAppRepo{db}
	ctx := context.Background()

	_, err := repo.GetByID(ctx, "nonexistent")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestAppRepo_List(t *testing.T) {
	db := openSchemaDB(t)
	repo := &sqliteAppRepo{db}
	ctx := context.Background()

	for _, name := range []string{"App A", "App B", "App C"} {
		a := &models.App{Name: name, Token: "tok-" + name, RateLimit: 100}
		if err := repo.Create(ctx, a); err != nil {
			t.Fatalf("Create %q: %v", name, err)
		}
	}

	apps, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(apps) != 3 {
		t.Errorf("List: got %d apps, want 3", len(apps))
	}
}

func TestAppRepo_Update(t *testing.T) {
	db := openSchemaDB(t)
	repo := &sqliteAppRepo{db}
	ctx := context.Background()

	app := &models.App{Name: "Original", Token: "tok-orig", RateLimit: 100}
	if err := repo.Create(ctx, app); err != nil {
		t.Fatalf("Create: %v", err)
	}

	app.Name = "Updated"
	app.RateLimit = 200
	if err := repo.Update(ctx, app); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.GetByID(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "Updated" {
		t.Errorf("Name after update: got %q, want %q", got.Name, "Updated")
	}
	if got.RateLimit != 200 {
		t.Errorf("RateLimit after update: got %d, want 200", got.RateLimit)
	}
}

func TestAppRepo_Delete(t *testing.T) {
	db := openSchemaDB(t)
	repo := &sqliteAppRepo{db}
	ctx := context.Background()

	app := &models.App{Name: "ToDelete", Token: "tok-del", RateLimit: 100}
	if err := repo.Create(ctx, app); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Delete(ctx, app.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := repo.GetByID(ctx, app.ID)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("expected sql.ErrNoRows after delete, got %v", err)
	}
}

// --- Event repository tests ---

func makeApp(t *testing.T, db *sqlx.DB, name, token string) *models.App {
	t.Helper()
	repo := &sqliteAppRepo{db}
	app := &models.App{Name: name, Token: token, RateLimit: 100}
	if err := repo.Create(context.Background(), app); err != nil {
		t.Fatalf("makeApp Create: %v", err)
	}
	return app
}

func TestEventRepo_CreateAndGetByID(t *testing.T) {
	db := openSchemaDB(t)
	repo := &sqliteEventRepo{db}
	ctx := context.Background()

	app := makeApp(t, db, "n8n", "tok-n8n")

	event := &models.Event{
		AppID:       app.ID,
		Severity:    "info",
		DisplayText: "Workflow completed",
		RawPayload:  models.RawMessage(`{"status":"ok"}`),
		Fields:      models.RawMessage(`{"duration_ms":42}`),
	}

	if err := repo.Create(ctx, event); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if event.ID == "" {
		t.Fatal("expected ID to be set after Create")
	}
	if event.ReceivedAt.IsZero() {
		t.Fatal("expected ReceivedAt to be set after Create")
	}

	got, err := repo.GetByID(ctx, event.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.AppID != app.ID {
		t.Errorf("AppID: got %q, want %q", got.AppID, app.ID)
	}
	if got.Severity != "info" {
		t.Errorf("Severity: got %q, want %q", got.Severity, "info")
	}
	if got.DisplayText != "Workflow completed" {
		t.Errorf("DisplayText: got %q, want %q", got.DisplayText, "Workflow completed")
	}
}

func TestEventRepo_List_FilterByAppAndSeverity(t *testing.T) {
	db := openSchemaDB(t)
	repo := &sqliteEventRepo{db}
	ctx := context.Background()

	app1 := makeApp(t, db, "App1", "tok-app1")
	app2 := makeApp(t, db, "App2", "tok-app2")

	severities := []string{"info", "warn", "error", "info"}
	for i, sev := range severities {
		appID := app1.ID
		if i == 3 {
			appID = app2.ID
		}
		e := &models.Event{AppID: appID, Severity: sev, DisplayText: "msg"}
		if err := repo.Create(ctx, e); err != nil {
			t.Fatalf("Create event: %v", err)
		}
	}

	// Filter by app1 only — should get 3 events.
	events, err := repo.List(ctx, EventFilter{AppID: app1.ID})
	if err != nil {
		t.Fatalf("List by app: %v", err)
	}
	if len(events) != 3 {
		t.Errorf("List by app1: got %d, want 3", len(events))
	}

	// Filter by app1 + severity=info — should get 1 event.
	events, err = repo.List(ctx, EventFilter{AppID: app1.ID, Severity: "info"})
	if err != nil {
		t.Fatalf("List by app+severity: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("List by app1+info: got %d, want 1", len(events))
	}
}

func TestEventRepo_List_LimitOffset(t *testing.T) {
	db := openSchemaDB(t)
	repo := &sqliteEventRepo{db}
	ctx := context.Background()

	app := makeApp(t, db, "Pager", "tok-pager")
	for i := 0; i < 5; i++ {
		e := &models.Event{AppID: app.ID, Severity: "debug", DisplayText: "msg"}
		if err := repo.Create(ctx, e); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	page, err := repo.List(ctx, EventFilter{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("List with limit/offset: %v", err)
	}
	if len(page) != 2 {
		t.Errorf("List limit=2 offset=1: got %d, want 2", len(page))
	}
}

func TestEventRepo_CountBySeverity(t *testing.T) {
	db := openSchemaDB(t)
	repo := &sqliteEventRepo{db}
	ctx := context.Background()

	app := makeApp(t, db, "Counter", "tok-counter")
	for _, sev := range []string{"info", "info", "warn", "error"} {
		e := &models.Event{AppID: app.ID, Severity: sev, DisplayText: "x"}
		if err := repo.Create(ctx, e); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	counts, err := repo.CountBySeverity(ctx, app.ID)
	if err != nil {
		t.Fatalf("CountBySeverity: %v", err)
	}
	if counts["info"] != 2 {
		t.Errorf("info count: got %d, want 2", counts["info"])
	}
	if counts["warn"] != 1 {
		t.Errorf("warn count: got %d, want 1", counts["warn"])
	}
	if counts["error"] != 1 {
		t.Errorf("error count: got %d, want 1", counts["error"])
	}
}

func TestEventRepo_DeleteOlderThan(t *testing.T) {
	db := openSchemaDB(t)
	eRepo := &sqliteEventRepo{db}
	ctx := context.Background()

	app := makeApp(t, db, "Cleaner", "tok-cleaner")

	// Insert event with a known timestamp via direct SQL to control received_at.
	old := &models.Event{AppID: app.ID, Severity: "debug", DisplayText: "old"}
	if err := eRepo.Create(ctx, old); err != nil {
		t.Fatalf("Create old: %v", err)
	}
	// Back-date the event so it appears older than the cutoff.
	_, err := db.ExecContext(ctx,
		`UPDATE events SET received_at = ? WHERE id = ?`,
		time.Now().UTC().Add(-48*time.Hour), old.ID,
	)
	if err != nil {
		t.Fatalf("backdate event: %v", err)
	}

	recent := &models.Event{AppID: app.ID, Severity: "info", DisplayText: "recent"}
	if err := eRepo.Create(ctx, recent); err != nil {
		t.Fatalf("Create recent: %v", err)
	}

	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	if err := eRepo.DeleteOlderThan(ctx, cutoff); err != nil {
		t.Fatalf("DeleteOlderThan: %v", err)
	}

	events, err := eRepo.List(ctx, EventFilter{AppID: app.ID})
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("after DeleteOlderThan: got %d events, want 1", len(events))
	}
	if events[0].ID != recent.ID {
		t.Errorf("remaining event should be recent, got ID %q", events[0].ID)
	}
}
